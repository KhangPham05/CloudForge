package controller

// phaseReady is the Status.Phase value every CRD in this project uses
// to mean "fully provisioned and usable." Shared across all three
// controllers rather than repeated as a literal in each one.
const phaseReady = "Ready"
