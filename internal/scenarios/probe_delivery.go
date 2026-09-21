package scenarios

import "fmt"

// probeDeliveryRun classifies recorded delivery attempts without starting a model session.
func probeDeliveryRun(root string) (map[string]any, error) {
	results := map[string]any{}
	for _, name := range deliveryFixtureNames {
		evidence, err := readDeliveryFixture(root, name)
		if err != nil {
			return nil, fmt.Errorf("delivery fixture %s: %w", name, err)
		}
		outcome, reasons := deliveryOracle(evidence)
		results[name] = map[string]any{"outcome": outcome, "reasons": toAnyList(reasons)}
	}
	return map[string]any{"fixtures": results}, nil
}

func probeDelivery(repo, root string) error {
	artifact, err := probeDeliveryRun(root)
	if err != nil {
		return err
	}
	return writeArtifact(repo, "delivery.json", artifact)
}
