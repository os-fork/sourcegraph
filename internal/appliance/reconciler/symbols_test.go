package reconciler

import "time"

func (suite *ApplianceTestSuite) TestDeploySymbols() {
	for _, tc := range []struct {
		name string
	}{
		{name: "symbols/default"},

		// This service does some logic on the storage quantity, so we can't
		// just rely on the standard config test for storage amounts/classes.
		{name: "symbols/with-storage"},
	} {
		suite.Run(tc.name, func() {
			namespace := suite.createConfigMap(tc.name)

			// Wait for reconciliation to be finished.
			suite.Require().Eventually(func() bool {
				return suite.getConfigMapReconcileEventCount(namespace) > 0
			}, time.Second*10, time.Millisecond*200)

			suite.makeGoldenAssertions(namespace, tc.name)
		})
	}
}
