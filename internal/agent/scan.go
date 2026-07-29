package agent

// Inventory is everything one sweep of this machine turned up: the sessions
// themselves, which tools were actually found, where each one lives, and which
// ones went wrong on the way.
//
// Problems is deliberately separate from an error return. A sweep that half
// worked is still worth showing - the alternative is a blank screen because one
// tool is mid-upgrade.
type Inventory struct {
	Sessions []Session
	Present  []ID
	Roots    map[ID]Roots
	Problems []string
}

// Scan gathers sessions from every coding tool installed on this machine.
//
// One tool failing is not allowed to blank the list. A broken or half-installed
// tool would otherwise hide everything else, which is the worst possible failure
// for a listing whose whole job is showing you your work - so a tool that errors
// contributes a line to Problems and nothing else.
//
// claudeConfigDir overrides where Claude Code is looked for and is passed to that
// provider alone: --config-dir has always meant the Claude directory, and handing
// it to another tool would point it at something unrelated.
//
// Lives here rather than in the TUI because the cockpit and `entangle gui` both
// show this same list, and two copies of this loop would drift - one of them
// growing support for a new tool while the other quietly kept omitting it.
func Scan(claudeConfigDir string) Inventory {
	inv := Inventory{Roots: map[ID]Roots{}}
	for _, p := range All() {
		override := ""
		if p.ID() == ClaudeCode {
			override = claudeConfigDir
		}
		r, installed, err := p.Locate(override)
		if err != nil || !installed {
			continue
		}
		got, err := p.ListSessions(r)
		if err != nil {
			inv.Problems = append(inv.Problems, p.DisplayName()+": "+err.Error())
			continue
		}
		inv.Roots[p.ID()] = r
		inv.Present = append(inv.Present, p.ID())
		inv.Sessions = append(inv.Sessions, got...)
	}
	SortSessions(inv.Sessions)
	// Each provider derived its handles from its own sessions alone, so two tools
	// can offer the same one. Redo them over the joined list: the handle on screen
	// is what a user reads out or pastes into the CLI.
	AssignShortIDs(inv.Sessions, ShortIDMin)
	return inv
}
