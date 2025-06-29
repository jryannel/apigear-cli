package sim

import (
	"fmt"

	"github.com/dop251/goja"
)

type World struct {
	engine         *Engine
	services       map[string]*ObjectService
	clients        map[string]*ObjectClient
	channels       map[string]*Channel
	servicesLoaded bool
	channelsLoaded bool
}

func NewWorld(engine *Engine) *World {
	log.Info().Msg("NewWorld")
	w := &World{
		engine:   engine,
		services: make(map[string]*ObjectService),
		clients:  make(map[string]*ObjectClient),
		channels: make(map[string]*Channel),
	}
	return w
}

func (w *World) CreateService(object string, properties map[string]any) (*ObjectService, error) {
	if w.channelsLoaded {
		return nil, fmt.Errorf("channels already loaded. Can not mix channels and services")
	}
	w.servicesLoaded = true
	return NewObjectService(w.engine, object, properties), nil
}

func (w *World) GetService(object string) *ObjectService {
	if w.services[object] == nil {
		return nil
	}
	return w.services[object]
}

func (w *World) register(rt *goja.Runtime) {
	if err := rt.Set("$createBareService", w.CreateService); err != nil {
		log.Error().Err(err).Msg("failed to set $createBareService")
	}
	if err := rt.Set("$getService", w.GetService); err != nil {
		log.Error().Err(err).Msg("failed to set $getService")
	}
	if err := rt.Set("$createChannel", w.CreateChannel); err != nil {
		log.Error().Err(err).Msg("failed to set $createChannel")
	}
	if err := rt.Set("$$getChannel", w.GetChannel); err != nil {
		log.Error().Err(err).Msg("failed to set $$getChannel")
	}
	if err := rt.Set("$quit", w.quit); err != nil {
		log.Error().Err(err).Msg("failed to set $quit")
	}
}

func (w *World) CreateChannel(url string) (*Channel, error) {
	if w.servicesLoaded {
		return nil, fmt.Errorf("services already loaded. Can not mix channels and services")
	}
	w.channelsLoaded = true
	if url == "" {
		url = "ws://localhost:5555/ws"
	}
	c, ok := w.channels[url]
	if ok {
		log.Warn().Msgf("channel %s already exists", url)
		return c, nil
	}
	c, err := NewChannel(w.engine, url)
	if err != nil {
		return nil, err
	}
	w.channels[url] = c
	return c, nil
}

func (w *World) GetChannel(url string) *Channel {
	if w.channels[url] == nil {
		log.Warn().Msgf("channel %s not found", url)
		return nil
	}
	return w.channels[url]
}

func (w *World) quit() {
	for _, c := range w.channels {
		if err := c.Disconnect(); err != nil {
			log.Error().Err(err).Msgf("failed to disconnect channel %s", c.url)
		}
	}
	w.engine.Close()
}
