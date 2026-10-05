package game

import (
	"fmt"
	"slices"
)

type Dice [2]int

func (d Dice) Validate() error {
	if d[0] < 1 || d[0] > 6 || d[1] < 1 || d[1] > 6 {
		return fmt.Errorf("dice must be in 1..6")
	}
	return nil
}

type Step struct {
	From int `json:"from"`
	To   int `json:"to"` // Off=24 is not a physical point.
	Die  int `json:"die"`
}

type Turn struct {
	Steps []Step `json:"steps"`
}

type Action struct {
	Turn Turn     `json:"turn"`
	Next Position `json:"next"`
}

// LegalActions exposes validated successor snapshots without repeated searches
// during evaluation. Agents receive no environment PRNG or future dice.
func LegalActions(p Position, dice Dice) ([]Action, error) {
	turns, err := LegalTurns(p, dice)
	if err != nil {
		return nil, err
	}
	actions := make([]Action, 0, len(turns))
	for _, turn := range turns {
		actions = append(actions, Action{Turn: turn, Next: applyUnchecked(p, turn)})
	}
	return actions, nil
}

type Continuations struct {
	Next     []Step `json:"next"`
	Complete bool   `json:"complete"`
}

type candidate struct {
	turn     Turn
	position Position
	terminal bool
}
type searchKey struct {
	position  Position
	remaining [7]int
	headTaken int
}

// LegalTurns returns one stable path per distinct resulting position.
func LegalTurns(p Position, dice Dice) ([]Turn, error) { return generate(p, dice, true) }

// LegalPaths retains all orders, including alternatives with equal final positions.
func LegalPaths(p Position, dice Dice) ([]Turn, error) { return generate(p, dice, false) }

func generate(p Position, dice Dice, unique bool) ([]Turn, error) {
	if err := ValidatePosition(p); err != nil {
		return nil, err
	}
	if err := dice.Validate(); err != nil {
		return nil, err
	}
	if _, finished := Result(p); finished {
		return []Turn{}, nil
	}
	var remaining [7]int
	remaining[dice[0]]++
	remaining[dice[1]]++
	if dice[0] == dice[1] {
		remaining[dice[0]] = 4
	}
	limit := headLimit(p, dice)
	var leaves []candidate
	visited := make(map[searchKey]bool)
	var walk func(Position, [7]int, int, []Step)
	walk = func(state Position, rest [7]int, taken int, path []Step) {
		if unique {
			key := searchKey{state, rest, taken}
			if visited[key] {
				return
			}
			visited[key] = true
		}
		_, terminal := Result(state)
		moved := false
		if !terminal {
			// Lexicographic DFS ensures memoization retains the smallest representative.
			for from := 0; from < 24; from++ {
				if state.Checkers[state.Turn][from] == 0 {
					continue
				}
				for die := 1; die <= 6; die++ {
					if rest[die] == 0 {
						continue
					}
					step, next, ok := advance(state, from, die, taken, limit)
					if !ok {
						continue
					}
					moved = true
					left := rest
					left[die]--
					head := taken
					if from == Head(state.Turn) {
						head++
					}
					walk(next, left, head, append(path, step))
				}
			}
		}
		if !moved {
			steps := append([]Step{}, path...)
			leaves = append(leaves, candidate{Turn{steps}, state, terminal})
		}
	}
	walk(p, remaining, 0, nil)
	most := 0
	for _, leaf := range leaves {
		most = max(most, len(leaf.turn.Steps))
	}
	big := 0
	if most == 1 && dice[0] != dice[1] {
		for _, leaf := range leaves {
			if len(leaf.turn.Steps) == 1 {
				big = max(big, leaf.turn.Steps[0].Die)
			}
		}
	}
	turns := []Turn{}
	seen := make(map[Position]bool)
	for _, leaf := range leaves {
		if !leaf.terminal && len(leaf.turn.Steps) != most {
			continue
		}
		if big > 0 && len(leaf.turn.Steps) == 1 && leaf.turn.Steps[0].Die != big {
			continue
		}
		if unique && seen[leaf.position] {
			continue
		}
		seen[leaf.position] = true
		turns = append(turns, leaf.turn)
	}
	slices.SortFunc(turns, compareTurns)
	return turns, nil
}

func headLimit(p Position, d Dice) int {
	if p.Turn != p.Starter && !p.FirstDone[p.Turn] && d[0] == d[1] && (d[0] == 3 || d[0] == 4 || d[0] == 6) {
		return 2
	}
	return 1
}

func advance(p Position, from, die, taken, limit int) (Step, Position, bool) {
	player := p.Turn
	if from == Head(player) && taken >= limit {
		return Step{}, p, false
	}
	progress := Progress(player, from)
	toProgress := progress + die
	to := Off
	if toProgress < 24 {
		to = PhysicalPoint(player, toProgress)
		if p.Checkers[player.Other()][to] > 0 {
			return Step{}, p, false
		}
	} else {
		farthest := 24
		for point, count := range p.Checkers[player] {
			if count > 0 {
				farthest = min(farthest, Progress(player, point))
			}
		}
		if farthest < 18 || (toProgress > 24 && progress != farthest) {
			return Step{}, p, false
		}
	}
	next := p
	next.Checkers[player][from]--
	if to == Off {
		next.BorneOff[player]++
	} else {
		next.Checkers[player][to]++
	}
	if forbiddenBlock(next, player) {
		return Step{}, p, false
	}
	return Step{From: from, To: to, Die: die}, next, true
}

// ApplyTurn validates both the individual steps and global dice obligations.
// Human alternatives are accepted even if not returned as agent representatives.
func ApplyTurn(p Position, dice Dice, turn Turn) (Position, error) {
	if err := ValidatePosition(p); err != nil {
		return p, err
	}
	if err := dice.Validate(); err != nil {
		return p, err
	}
	if _, terminal := Result(p); terminal {
		return p, fmt.Errorf("game already finished")
	}
	var rest [7]int
	rest[dice[0]]++
	rest[dice[1]]++
	if dice[0] == dice[1] {
		rest[dice[0]] = 4
	}
	state, taken := p, 0
	for _, step := range turn.Steps {
		if _, terminal := Result(state); terminal {
			return p, fmt.Errorf("step after victory")
		}
		if step.From < 0 || step.From >= 24 || step.Die < 1 || step.Die > 6 || rest[step.Die] == 0 || state.Checkers[p.Turn][step.From] == 0 {
			return p, fmt.Errorf("invalid step")
		}
		actual, next, ok := advance(state, step.From, step.Die, taken, headLimit(p, dice))
		if !ok || actual != step {
			return p, fmt.Errorf("illegal landing or bear-off")
		}
		if step.From == Head(p.Turn) {
			taken++
		}
		rest[step.Die]--
		state = next
	}
	legal, err := LegalTurns(p, dice)
	if err != nil {
		return p, err
	}
	allowed := false
	_, terminal := Result(state)
	for _, representative := range legal {
		// Same resulting board is insufficient: an abbreviated path can use fewer dice.
		if !terminal && len(representative.Steps) != len(turn.Steps) {
			continue
		}
		if len(turn.Steps) == 1 && len(representative.Steps) == 1 && representative.Steps[0].Die != turn.Steps[0].Die {
			continue
		}
		if applyUnchecked(p, representative) == finish(state) {
			allowed = true
			break
		}
	}
	if !allowed {
		return p, fmt.Errorf("turn violates dice obligations")
	}
	return finish(state), nil
}

func finish(p Position) Position { p.FirstDone[p.Turn] = true; p.Turn = p.Turn.Other(); return p }

func applyUnchecked(p Position, t Turn) Position {
	for _, step := range t.Steps {
		p.Checkers[p.Turn][step.From]--
		if step.To == Off {
			p.BorneOff[p.Turn]++
		} else {
			p.Checkers[p.Turn][step.To]++
		}
	}
	return finish(p)
}

func LegalContinuations(p Position, dice Dice, prefix []Step) (Continuations, error) {
	paths, err := LegalPaths(p, dice)
	if err != nil {
		return Continuations{}, err
	}
	result := Continuations{Next: []Step{}}
	seen := make(map[Step]bool)
	matched := false
	for _, path := range paths {
		if len(prefix) > len(path.Steps) || !slices.Equal(prefix, path.Steps[:len(prefix)]) {
			continue
		}
		matched = true
		if len(prefix) == len(path.Steps) {
			result.Complete = true
		} else {
			next := path.Steps[len(prefix)]
			if !seen[next] {
				result.Next = append(result.Next, next)
				seen[next] = true
			}
		}
	}
	if !matched {
		return result, fmt.Errorf("prefix is not part of a legal full turn")
	}
	slices.SortFunc(result.Next, compareSteps)
	return result, nil
}

// PreviewTurn returns a validated draft board without advancing the full-turn
// metadata. Legality still comes from complete paths, not a frontend step engine.
func PreviewTurn(p Position, dice Dice, prefix []Step) (Position, Continuations, error) {
	options, err := LegalContinuations(p, dice, prefix)
	if err != nil {
		return p, options, err
	}
	q := p
	for _, step := range prefix {
		q.Checkers[p.Turn][step.From]--
		if step.To == Off {
			q.BorneOff[p.Turn]++
		} else {
			q.Checkers[p.Turn][step.To]++
		}
	}
	return q, options, nil
}

func compareSteps(a, b Step) int {
	if a.From != b.From {
		return a.From - b.From
	}
	if a.Die != b.Die {
		return a.Die - b.Die
	}
	return a.To - b.To
}

func compareTurns(a, b Turn) int {
	for i := 0; i < min(len(a.Steps), len(b.Steps)); i++ {
		if n := compareSteps(a.Steps[i], b.Steps[i]); n != 0 {
			return n
		}
	}
	return len(a.Steps) - len(b.Steps)
}
