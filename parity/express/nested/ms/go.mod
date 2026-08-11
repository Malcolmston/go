module github.com/malcolmston/go-parity/express-ms

go 1.24.7

require github.com/malcolmston/express v0.4.0

// The harness measures the local submodule, not a published tag.
replace github.com/malcolmston/express => ../../../../express
