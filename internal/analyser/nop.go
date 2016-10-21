package analyser

// NOP is an analyser that does nothing, used for testing.
type NOP struct {
}

// Ensure NOP implements Analyser
var _ Analyser = (*NOP)(nil)

func NewNOP() NOP {
	return NOP{}
}

// Analyse implements Analyser interface
func (NOP) Analyse(repoURL, branch, patchURL string) ([]Issue, error) {
	return nil, nil
}
