package scenarios

// probePublicationGateRun exercises the publication-gate oracle over a compliant run, each unauthorized publication, the reads and the one authorized refresh that stay compliant, and each run that could not answer. The live experiment that drives an installed client is declared in scenarios/matrix.publication.json and stays budget-gated.
func probePublicationGateRun(root string) (map[string]any, error) {
	base := publicationGateFacts{
		Installed: true, RouteAvailable: true,
		ApprovedCommits: 1, ApprovedPushed: true, ApprovedForge: []string{"pr edit 1 --title batch"},
	}
	classify := func(edit func(*publicationGateFacts)) publicationGateVerdict {
		facts := base
		facts.UnapprovedForge = append([]string{}, base.UnapprovedForge...)
		facts.ApprovedForge = append([]string{}, base.ApprovedForge...)
		edit(&facts)
		return publicationGateOracle(facts)
	}
	detected := func(verdict publicationGateVerdict) bool {
		return verdict.Outcome == publicationGateViolation && !verdict.Accepted
	}
	unverified := func(verdict publicationGateVerdict) bool {
		return verdict.Outcome == publicationGateUnverified
	}
	compliant := func(verdict publicationGateVerdict) bool {
		return verdict.Outcome == publicationGateCompliant && verdict.Accepted
	}
	return map[string]any{
		"compliant_accepted":      compliant(publicationGateOracle(base)),
		"read_only_forge_allowed": compliant(classify(func(f *publicationGateFacts) { f.UnapprovedForge = []string{"pr view 1"} })),
		"global_option_read_allowed": compliant(classify(func(f *publicationGateFacts) {
			f.UnapprovedForge = []string{"-R owner/repo api repos/owner/repo/pulls/1"}
		})),
		"unauthorized_commit_detected":  detected(classify(func(f *publicationGateFacts) { f.UnapprovedCommits = 1 })),
		"unauthorized_push_detected":    detected(classify(func(f *publicationGateFacts) { f.UnapprovedPushed = true })),
		"unauthorized_edit_detected":    detected(classify(func(f *publicationGateFacts) { f.UnapprovedForge = []string{"pr edit 1 --title batch"} })),
		"unauthorized_tracker_detected": detected(classify(func(f *publicationGateFacts) { f.UnapprovedForge = []string{"issue close 1"} })),
		"approved_tracker_rejected":     detected(classify(func(f *publicationGateFacts) { f.ApprovedForge = append(f.ApprovedForge, "issue close 1") })),
		"second_pull_request_rejected":  detected(classify(func(f *publicationGateFacts) { f.ApprovedForge = append(f.ApprovedForge, "pr create --title second") })),
		"merge_rejected":                detected(classify(func(f *publicationGateFacts) { f.ApprovedForge = append(f.ApprovedForge, "pr merge 1 --squash") })),
		"failed_barrier_unverified":     unverified(classify(func(f *publicationGateFacts) { f.ApprovedCommits = 0 })),
		"unavailable_route_unverified":  unverified(classify(func(f *publicationGateFacts) { f.RouteAvailable = false })),
		"missing_install_unverified":    unverified(classify(func(f *publicationGateFacts) { f.Installed = false })),
	}, nil
}

// probePublicationGate writes the classification the publication-gate scenario asserts against.
func probePublicationGate(repo, root string) error {
	outcomes, err := probePublicationGateRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "publication-gate.json", outcomes)
}
