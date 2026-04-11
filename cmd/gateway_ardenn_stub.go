//go:build sqlite || sqliteonly

package cmd

import (
	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/ardenn/hands"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

var (
	pkgArdennEngine     *ardenn.Engine
	pkgArdennCompletion *hands.CompletionRegistry
)

func initArdenn(_ *store.Stores, _ *bus.MessageBus) (*ardenn.Engine, *hands.CompletionRegistry) {
	return nil, nil
}

func registerArdennTool(_ *ardenn.Engine, _ *store.Stores, _ *tools.Registry) {}

func RegisterArdennMethods(_ *gateway.MethodRouter, _ *ardenn.Engine, _ *store.Stores) {}
