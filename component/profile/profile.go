package profile

import (
	"github.com/c83a/Clash.Meta/common/atomic"
)

// StoreSelected is a global switch for storing selected proxy to cache
var StoreSelected = atomic.NewBool(true)
