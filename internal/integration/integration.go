// Package integration carries the handful of cats-integration constants
// cats-todo needs at runtime. It is a client-side sliver of cats'
// internal/integration package (github.com/rohanthewiz/cats): only the wire/env
// contract lives here, none of the installer machinery.
package integration

// CatsPaneIDEnvVar names the env var carrying the public pane id into spawned
// pane processes; cats-todo reads it to know which pane it is running in so
// drops can default to "this pane's session". The value is the compatibility
// contract with the cats server — never rename it independently.
const CatsPaneIDEnvVar = "CATS_PANE_ID"
