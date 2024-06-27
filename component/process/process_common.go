//go:build !(android && cmfa)

package process

import "github.com/c83a/Clash.Meta/constant"

func FindPackageName(metadata *constant.Metadata) (string, error) {
	return "", nil
}
