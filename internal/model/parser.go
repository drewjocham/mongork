package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/drewjocham/mongork/internal/jsonutil"
)

var (
	ErrTypeFieldRequired = errors.New("type field path is required")
	ErrTypeFieldNotFound = errors.New("type field not found")
	ErrTypeFieldNotStr   = errors.New("type field is not a string")
	ErrNoRegistryEntry   = errors.New("no registry entry for type")
)

type Cleaner func(map[string]any) map[string]any

type Option func(*parser)

type parser struct {
	cleaner Cleaner
}

func WithCleaner(cleaner Cleaner) Option {
	return func(p *parser) {
		p.cleaner = cleaner
	}
}

func Parse[T any](raw []byte, opts ...Option) (*T, error) {
	var out T
	if err := ParseInto(raw, &out, opts...); err != nil {
		return nil, err
	}
	return &out, nil
}

func ParseInto(raw []byte, out any, opts ...Option) error {
	p := parser{cleaner: DefaultCleaner}
	for _, opt := range opts {
		opt(&p)
	}

	var m map[string]any
	if err := jsonutil.Unmarshal(raw, &m); err != nil {
		return err
	}
	if p.cleaner != nil {
		m = p.cleaner(m)
	}
	buf, err := jsonutil.Marshal(m)
	if err != nil {
		return err
	}
	return jsonutil.Unmarshal(buf, out)
}

func ParseByType(raw []byte, fieldPath string, registry Registry, opts ...Option) (any, error) {
	if fieldPath == "" {
		return nil, ErrTypeFieldRequired
	}

	p := parser{cleaner: DefaultCleaner}
	for _, opt := range opts {
		opt(&p)
	}

	var m map[string]any
	if err := jsonutil.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if p.cleaner != nil {
		m = p.cleaner(m)
	}

	val, ok := valueAtPath(m, fieldPath)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrTypeFieldNotFound, fieldPath)
	}

	kind, ok := val.(string)
	if !ok || kind == "" {
		return nil, fmt.Errorf("%w: %q", ErrTypeFieldNotStr, fieldPath)
	}

	ctor := registry[strings.ToLower(kind)]
	if ctor == nil {
		return nil, fmt.Errorf("%w: %q", ErrNoRegistryEntry, kind)
	}

	instance := ctor()
	buf, err := jsonutil.Marshal(m)
	if err != nil {
		return nil, err
	}
	if err := jsonutil.Unmarshal(buf, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func valueAtPath(m map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	current := any(m)
	parts := strings.Split(path, ".")
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := obj[part]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}
