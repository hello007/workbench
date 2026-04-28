package main

import (
	"context"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	println("Git Manager starting...")
}

func (a *App) shutdown(ctx context.Context) {
	println("Git Manager shutting down...")
}
