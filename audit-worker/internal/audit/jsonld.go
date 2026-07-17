package audit

import (
	"encoding/json"
	"errors"
	"fmt"
)

// validJSONLD reports whether a script block is usable structured data.
//
// Two things make JSON-LD useless to a crawler: it does not parse, or it
// parses but declares nothing. Both are silent failures -- the page looks
// fine, and the rich result simply never appears -- so both are worth naming.
func validJSONLD(raw string) error {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return errors.New("it isn't valid JSON")
	}

	switch typed := value.(type) {
	case map[string]any:
		return validJSONLDObject(typed)

	case []any:
		// A top-level array of entities is legal.
		if len(typed) == 0 {
			return errors.New("it's an empty list")
		}
		for i, item := range typed {
			object, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("item %d isn't an object", i+1)
			}
			if err := validJSONLDObject(object); err != nil {
				return fmt.Errorf("item %d: %w", i+1, err)
			}
		}
		return nil

	default:
		return errors.New("it isn't an object or a list")
	}
}

func validJSONLDObject(object map[string]any) error {
	// @context is what makes JSON-LD linked data rather than arbitrary JSON.
	if _, ok := object["@context"]; !ok {
		// A @graph carries its context on the wrapper.
		if _, hasGraph := object["@graph"]; !hasGraph {
			return errors.New("it has no @context")
		}
	}

	if graph, ok := object["@graph"].([]any); ok {
		if len(graph) == 0 {
			return errors.New("its @graph is empty")
		}
		for i, item := range graph {
			entity, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("@graph item %d isn't an object", i+1)
			}
			if _, ok := entity["@type"]; !ok {
				return fmt.Errorf("@graph item %d has no @type", i+1)
			}
		}
		return nil
	}

	// Without @type nothing knows what the entity is, so nothing can use it.
	if _, ok := object["@type"]; !ok {
		return errors.New("it has no @type")
	}

	return nil
}
