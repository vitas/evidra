package evidra

import "io/fs"

// UIDistFS is the embedded UI filesystem. It is nil when the binary is built
// without the embed_ui tag (e.g. during tests or non-UI builds).
//
// Build with UI:
//
//	cd ui && npm run build
//	cd .. && go build -tags embed_ui -o bin/evidra-api ./cmd/evidra-api
var UIDistFS fs.FS
