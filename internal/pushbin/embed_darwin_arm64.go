//go:build darwin && arm64

package pushbin

import _ "embed"

//go:embed push_images_darwin_arm64.bin
var embeddedPushBinary []byte
