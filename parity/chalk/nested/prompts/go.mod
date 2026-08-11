module github.com/malcolmston/go-parity/chalk-prompts

go 1.24.7

require github.com/malcolmston/chalk v0.4.0

require (
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/term v0.17.0 // indirect
)

// The harness scores the working tree, not the last tag: a fix made in response
// to a mismatch has to be visible to the next run. The submodule cuts a new tag
// and drops this replace when the fixes ship.
replace github.com/malcolmston/chalk => ../../../../chalk
