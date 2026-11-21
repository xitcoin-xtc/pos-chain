package evmd

// EnableGovernanceProposalSubmissionForTests makes apps created afterwards in
// this process accept executable governance proposals in the ante handler and
// the governance precompile. Only test code calls it: the release binary has
// no flag, app.toml key or other path that reaches it, so proposal submission
// is disabled on every node and is not a node-local consensus option.
func EnableGovernanceProposalSubmissionForTests(enabled bool) {
	governanceProposalSubmissionForTests.Store(enabled)
}
