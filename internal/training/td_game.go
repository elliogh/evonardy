package training

import (
	"context"
	"fmt"
	"math"

	"evonardy/internal/agent"
	"evonardy/internal/encoder"
	"evonardy/internal/game"
	"evonardy/internal/neural"
	"evonardy/internal/random"
	"evonardy/internal/replay"
)

type TDConfig struct {
	Seed     uint64  `json:"seed"`
	Games    int     `json:"games"`
	MaxTurns int     `json:"max_turns"`
	Alpha    float64 `json:"alpha"`
	Epsilon  float64 `json:"epsilon"`
}

func DefaultTDConfig() TDConfig {
	return TDConfig{Seed: 42, Games: 32, MaxTurns: 1200, Alpha: .001, Epsilon: .05}
}

func (c TDConfig) Validate() error {
	if c.Games < 1 || c.Games > 200000 || c.MaxTurns < 1 || c.MaxTurns > replay.MaxEvents || int64(c.Games)*int64(c.MaxTurns) > 200000000 {
		return fmt.Errorf("TD requires 1..200000 games, 1..10000 turns, and at most 200000000 turn slots")
	}
	if !finiteTD(c.Alpha) || c.Alpha <= 0 || c.Alpha > 1 || !finiteTD(c.Epsilon) || c.Epsilon < 0 || c.Epsilon > 1 {
		return fmt.Errorf("alpha in (0,1] and epsilon in [0,1] must be finite")
	}
	return nil
}

type TDMetric struct {
	Game               int           `json:"game"`
	Status             string        `json:"status"`
	Outcome            *game.Outcome `json:"outcome"`
	Decisions          int           `json:"decisions"`
	ForwardEvaluations int           `json:"forward_evaluations"`
	Updates            int           `json:"updates"`
	MeanAbsDelta       float64       `json:"mean_abs_delta"`
}

// TDState is retained only at game boundaries: no partly updated game is committed.
type TDState struct {
	Version        int                    `json:"version"`
	Algorithm      string                 `json:"algorithm"`
	Ruleset        string                 `json:"ruleset"`
	EncoderVersion string                 `json:"encoder_version"`
	NetworkVersion string                 `json:"network_version"`
	RandomContract string                 `json:"random_contract"`
	Config         TDConfig               `json:"config"`
	Parameters     agent.NeuralParameters `json:"parameters"`
	Games          int                    `json:"games"`
	History        []TDMetric             `json:"history"`
	Counters       Counters               `json:"counters"`
}

func NewTD(c TDConfig) (TDState, error) {
	if err := c.Validate(); err != nil {
		return TDState{}, err
	}
	s := TDState{Version: 1, Algorithm: TDAlgorithm, Ruleset: game.Ruleset, EncoderVersion: encoder.Version, NetworkVersion: neural.Version, RandomContract: TDRandomContract, Config: c, History: []TDMetric{}}
	copy(s.Parameters[:], neural.Initialize(c.Seed, 0).Parameters())
	return s, nil
}

// TrainTDGame runs one sequential self-play game with the current network on both
// sides. Failure/cancellation returns the original state, not partial updates.
func TrainTDGame(ctx context.Context, s TDState) (TDState, PlayedGame, error) {
	if err := ValidateTD(s); err != nil {
		return s, PlayedGame{}, err
	}
	if s.Games >= s.Config.Games {
		return s, PlayedGame{}, fmt.Errorf("TD game budget exhausted")
	}
	learner, err := NewTDLearner(s.Parameters[:], s.Config.Alpha)
	if err != nil {
		return s, PlayedGame{}, err
	}
	label := fmt.Sprintf("td/game/%d", s.Games)
	opening := []game.Dice{}
	starter := game.White
	for attempt := uint64(0); ; attempt++ {
		if err := ctx.Err(); err != nil {
			return s, PlayedGame{}, err
		}
		if attempt >= replay.MaxEvents {
			return s, PlayedGame{}, fmt.Errorf("TD opening retry limit reached")
		}
		r := random.New(s.Config.Seed, label+"/opening", attempt)
		dice := game.Dice{r.IntN(6) + 1, r.IntN(6) + 1}
		opening = append(opening, dice)
		if dice[0] != dice[1] {
			if dice[1] > dice[0] {
				starter = game.Black
			}
			break
		}
	}
	p := game.Initial(starter)
	learnerID := TDAlgorithm + "/self-play"
	r := replay.New(p, opening, [2]string{learnerID, learnerID}, s.Config.Seed, uint64(s.Games), s.Config.MaxTurns)
	metric := TDMetric{Game: s.Games + 1, Status: replay.Truncated}
	var sideTurns [2]uint64
	for turn := 0; turn < s.Config.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return s, PlayedGame{}, err
		}
		player := p.Turn
		dice := opening[len(opening)-1]
		if turn > 0 {
			d := random.New(s.Config.Seed, fmt.Sprintf("%s/dice/%d", label, player), sideTurns[player])
			dice = game.Dice{d.IntN(6) + 1, d.IntN(6) + 1}
		}
		exploration := random.New(s.Config.Seed, fmt.Sprintf("%s/exploration/%d", label, player), sideTurns[player])
		sideTurns[player]++
		actions, err := game.LegalActions(p, dice)
		if err != nil {
			return s, PlayedGame{}, err
		}
		choice, evaluations := 0, 0
		if float64(exploration.Uint64()>>11)*(1.0/(1<<53)) < s.Config.Epsilon {
			choice, err = agent.NewRandom(exploration).Choose(ctx, p, dice, actions)
		} else {
			bot, createErr := agent.NewNeural(learner.Parameters())
			if createErr != nil {
				return s, PlayedGame{}, createErr
			}
			choice, evaluations, err = bot.ChooseMeasured(ctx, p, dice, actions)
		}
		if err != nil || choice < 0 || choice >= len(actions) {
			return s, PlayedGame{}, fmt.Errorf("TD action selection failed: %v", err)
		}
		action := actions[choice]
		next, err := game.ApplyTurn(p, dice, action.Turn)
		if err != nil || next != action.Next {
			return s, PlayedGame{}, fmt.Errorf("TD successor/application mismatch: %v", err)
		}
		update, err := learner.Update(p, next)
		if err != nil {
			return s, PlayedGame{}, err
		}
		metric.Decisions++
		metric.Updates++
		metric.ForwardEvaluations += evaluations + update.ForwardEvaluations
		metric.MeanAbsDelta += math.Abs(update.Delta)
		r.Append(dice, action.Turn, next)
		p = next
		if update.Terminal {
			metric.Status = replay.Completed
			break
		}
	}
	r.Finish(p, metric.Status)
	metric.Outcome = r.Outcome
	metric.MeanAbsDelta /= float64(metric.Decisions)
	next := s
	copy(next.Parameters[:], learner.Parameters())
	next.Games++
	next.History = append(append([]TDMetric{}, s.History...), metric)
	next.Counters.Games++
	if metric.Status == replay.Completed {
		next.Counters.CompletedGames++
	} else {
		next.Counters.TruncatedGames++
	}
	next.Counters.Decisions += uint64(metric.Decisions)
	next.Counters.ForwardEvaluations += uint64(metric.ForwardEvaluations)
	next.Counters.Updates += uint64(metric.Updates)
	played := PlayedGame{Generation: next.Games, Index: s.Games, CandidateID: learnerID, OpponentID: learnerID, CandidateSide: game.Player(s.Games % 2), Replay: r}
	return next, played, nil
}

func ValidateTD(s TDState) error {
	if s.Version != 1 || s.Algorithm != TDAlgorithm || s.Ruleset != game.Ruleset || s.EncoderVersion != encoder.Version || s.NetworkVersion != neural.Version || s.RandomContract != TDRandomContract {
		return fmt.Errorf("incompatible TD state")
	}
	if err := s.Config.Validate(); err != nil {
		return err
	}
	if _, err := NewTDLearner(s.Parameters[:], s.Config.Alpha); err != nil {
		return err
	}
	if s.Games < 0 || s.Games > s.Config.Games || len(s.History) != s.Games {
		return fmt.Errorf("invalid TD game history")
	}
	var counters Counters
	for i, metric := range s.History {
		if metric.Game != i+1 || metric.Decisions < 1 || metric.Decisions > s.Config.MaxTurns || metric.Updates != metric.Decisions || metric.ForwardEvaluations < metric.Updates || !finiteTD(metric.MeanAbsDelta) || metric.MeanAbsDelta < 0 || metric.MeanAbsDelta > 2 {
			return fmt.Errorf("invalid TD metric")
		}
		switch metric.Status {
		case replay.Completed:
			if metric.Outcome == nil || !metric.Outcome.Winner.Valid() {
				return fmt.Errorf("missing TD terminal outcome")
			}
			counters.CompletedGames++
		case replay.Truncated:
			if metric.Outcome != nil || metric.Decisions != s.Config.MaxTurns {
				return fmt.Errorf("invalid TD truncation")
			}
			counters.TruncatedGames++
		default:
			return fmt.Errorf("invalid TD outcome status")
		}
		counters.Games++
		counters.Decisions += uint64(metric.Decisions)
		counters.ForwardEvaluations += uint64(metric.ForwardEvaluations)
		counters.Updates += uint64(metric.Updates)
	}
	if counters != s.Counters {
		return fmt.Errorf("TD counters/history mismatch")
	}
	return nil
}
