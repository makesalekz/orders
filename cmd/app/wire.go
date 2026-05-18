//go:build wireinject
// +build wireinject

package main

import (
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	"github.com/makesalekz/orders/internal/biz"
	"github.com/makesalekz/orders/internal/conf"
	"github.com/makesalekz/orders/internal/data"
	"github.com/makesalekz/orders/internal/server"
	"github.com/makesalekz/orders/internal/service"
)

func wireApp(*conf.Bootstrap, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(server.ProviderSet, data.ProviderSet, biz.ProviderSet, service.ProviderSet, newApp))
}
