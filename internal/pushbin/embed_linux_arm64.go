//go:build linux && arm64

package pushbin

import _ "embed"

//go:embed push_images_linux_arm64.bin
var embeddedPushBinary []byte
