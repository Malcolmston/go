module github.com/malcolmston/go-parity/axios

go 1.24

require github.com/malcolmston/axios v0.3.0

// Point at the local submodule working tree so the parity harness measures the
// in-progress port rather than the published v0.3.0. The orchestrator publishes
// a new tag and drops this replace when the fixes ship.
replace github.com/malcolmston/axios => ../../axios
