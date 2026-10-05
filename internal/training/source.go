package training

import (
	"encoding/hex"
	"fmt"
	"math"

	"evonardy/internal/agent"
)

const FromModelAlgorithm = "ga-linear-from-model-v1"
const FromModelRandomContract = "keyed-pcg-ga-from-model-v1"

// LinearSource is frozen at job creation; resuming never reloads its package.
type LinearSource struct {
	Policy      agent.Policy `json:"policy"`
	ModelSHA256 string       `json:"model_sha256"`
}

func NewFromModel(c Config, source LinearSource) (State, error) {
	return newLinear(c, &source)
}

func validateSource(s State) error {
	if s.Source == nil {
		if s.Algorithm != Algorithm || s.RandomContract != RandomContract {
			return fmt.Errorf("incompatible linear training contract")
		}
		return nil
	}
	if s.Algorithm != FromModelAlgorithm || s.RandomContract != FromModelRandomContract {
		return fmt.Errorf("incompatible saved-source training contract")
	}
	for _, hash := range []string{s.Source.Policy.ID, s.Source.ModelSHA256} {
		b, err := hex.DecodeString(hash)
		if err != nil || len(b) != 32 || hash != hex.EncodeToString(b) {
			return fmt.Errorf("invalid source identity or model hash")
		}
	}
	if s.Source.Policy.Kind != "linear" {
		return fmt.Errorf("source must be a saved linear policy")
	}
	if err := s.Source.Policy.Validate(); err != nil {
		return err
	}
	for _, w := range s.Source.Policy.Weights {
		if math.Abs(w) > 10 {
			return fmt.Errorf("source weights exceed the GA genome bounds")
		}
	}
	games := int64(s.Config.Population) * int64(s.Config.Generations) * int64(s.Config.PairsPerOpponent) * 6
	if games > 200000 || games*int64(s.Config.MaxTurns) > 200000000 {
		return fmt.Errorf("three-opponent run exceeds 200000 games or 200000000 turn slots")
	}
	return nil
}

func GamesPerCandidate(s State) int {
	opponents := 2
	if s.Source != nil {
		opponents++
	}
	return s.Config.PairsPerOpponent * opponents * 2
}

func SelectionOpponent(s State, index int) agent.Policy {
	if index == 2 && s.Source != nil {
		return s.Source.Policy
	}
	return s.Development[index]
}

// generation is zero-based. Legacy jobs keep their original fixed schedule.
func SelectionPairSeed(s State, generation, opponent, pair int) uint64 {
	domain := "ga/development"
	if s.Source != nil {
		domain = fmt.Sprintf("ga/from-model/development/generation/%d", generation)
	}
	return PairSeed(s.Config.Seed, domain, opponent, pair)
}
