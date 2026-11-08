package report

// Team holds one team's game scores. Scores is empty until the team plays
// its first game.
type Team struct {
	Name   string
	Scores []int
}

// Weekly is one row of the weekly report.
type Weekly struct {
	Name  string
	Games int
	Mean  int
}

// Build returns one report row per team.
func Build(teams []Team) []Weekly {
	out := make([]Weekly, 0, len(teams))
	for _, t := range teams {
		out = append(out, Weekly{Name: t.Name, Games: len(t.Scores), Mean: average(t.Scores)})
	}
	return out
}

func average(scores []int) int {
	sum := 0
	for _, s := range scores {
		sum += s
	}
	return sum / len(scores)
}
