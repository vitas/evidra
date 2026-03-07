package e2e_test

import "testing"

func TestE2ERequiresBuildTag(t *testing.T) {
	t.Skip("e2e scenarios require -tags=e2e")
}
