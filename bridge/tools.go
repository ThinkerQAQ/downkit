//go:build tools

package downkit

// Keep gomobile's binding package pinned in go.mod. gomobile invokes gobind in
// a separate process, so the dependency is not visible from normal imports.
import _ "golang.org/x/mobile/bind"
