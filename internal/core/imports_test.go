package core

// This package is deliberately a leaf. A dependency from core to engine,
// harness, or adapters would create an import cycle or invert the contract
// boundary and is rejected by the Go compiler.
