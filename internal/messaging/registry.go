package messaging

import "fmt"

type Registry struct {
	platforms map[string]MessagingPlatform
}

func NewRegistry() *Registry {
	return &Registry{
		platforms: map[string]MessagingPlatform{},
	}
}

func (r *Registry) Register(platform MessagingPlatform) error {
	if platform == nil {
		return fmt.Errorf("register platform: nil platform")
	}

	name := platform.Platform()
	if name == "" {
		return fmt.Errorf("register platform: empty platform name")
	}
	if _, exists := r.platforms[name]; exists {
		return fmt.Errorf("register platform %s: already registered", name)
	}

	r.platforms[name] = platform
	return nil
}

func (r *Registry) Get(name string) (MessagingPlatform, error) {
	platform, ok := r.platforms[name]
	if !ok {
		return nil, fmt.Errorf("platform %s not registered", name)
	}

	return platform, nil
}
