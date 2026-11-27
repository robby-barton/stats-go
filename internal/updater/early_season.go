package updater

import "time"

// nowFunc is the observed snapshot clock for the opt-in FBS early-season
// blend. Production reads the real clock exactly once per enabled FBS input
// load; tests substitute a fake. There is deliberately no operator flag that
// can override any clock read.
var nowFunc = time.Now //nolint:gochecknoglobals // hermetic fake-clock seam for tests
