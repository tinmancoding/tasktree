package app

import (
	"fmt"
	"sort"

	"github.com/tinmancoding/tasktree/internal/domain"
	"github.com/tinmancoding/tasktree/internal/fsx"
	"github.com/tinmancoding/tasktree/internal/metadata"
)

// AnnotationEntry is a single annotation key/value pair, used for sorted list output.
type AnnotationEntry struct {
	Key   string
	Value string
}

// AnnotateService manages the annotations map in a tasktree's metadata.
type AnnotateService struct {
	store metadata.Store
}

func NewAnnotateService(store metadata.Store) AnnotateService {
	return AnnotateService{store: store}
}

// Set adds or overwrites the annotation with the given key. The key is
// validated via domain.ValidateAnnotationKey before any file is written.
func (s AnnotateService) Set(start, key, value string) error {
	if err := domain.ValidateAnnotationKey(key); err != nil {
		return err
	}
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}
	if spec.Metadata.Annotations == nil {
		spec.Metadata.Annotations = make(map[string]string)
	}
	spec.Metadata.Annotations[key] = value
	if err := s.store.Save(root, spec); err != nil {
		return fmt.Errorf("save metadata: %w", err)
	}
	return nil
}

// Unset removes the annotation with the given key. It is a no-op when the
// key does not exist, making it safe for use in scripts.
func (s AnnotateService) Unset(start, key string) error {
	if err := domain.ValidateAnnotationKey(key); err != nil {
		return err
	}
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}
	delete(spec.Metadata.Annotations, key)
	if err := s.store.Save(root, spec); err != nil {
		return fmt.Errorf("save metadata: %w", err)
	}
	return nil
}

// List returns all annotations sorted by key. Returns an empty (non-nil)
// slice when no annotations are set.
func (s AnnotateService) List(start string) ([]AnnotationEntry, error) {
	root, err := fsx.ResolveTasktreeRoot(start)
	if err != nil {
		return nil, err
	}
	spec, err := s.store.Load(root)
	if err != nil {
		return nil, fmt.Errorf("load metadata: %w", err)
	}
	entries := make([]AnnotationEntry, 0, len(spec.Metadata.Annotations))
	for k, v := range spec.Metadata.Annotations {
		entries = append(entries, AnnotationEntry{Key: k, Value: v})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries, nil
}
