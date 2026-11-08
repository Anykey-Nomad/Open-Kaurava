package screenui

import "strings"

// All returns every widget of the screen in document order (depth-first, the
// order the game draws them in), without the tree root. Interpretation code
// walks this list instead of naming panels one by one: .screen already says
// what the HUD is made of and where each piece sits.
func (s *Screen) All() []*Widget {
	if s == nil || s.Widgets == nil {
		return nil
	}
	var out []*Widget
	var walk func(*Widget)
	walk = func(w *Widget) {
		for _, child := range w.Children {
			if child == nil || child.IsArt {
				// Арт-узлы владельца уже возвращает его TextureRects.
				continue
			}
			out = append(out, child)
			walk(child)
		}
	}
	walk(s.Widgets)
	return out
}

// WidgetsNamed returns widgets whose name starts with prefix, in document
// order. The command grid uses it for CommandIcon01..12.
func (s *Screen) WidgetsNamed(prefix string) []*Widget {
	prefix = strings.ToLower(prefix)
	var out []*Widget
	for _, widget := range s.All() {
		if strings.HasPrefix(strings.ToLower(widget.Name), prefix) {
			out = append(out, widget)
		}
	}
	return out
}

// ChildrenOf returns the direct children of the first widget with this name.
func (s *Screen) ChildrenOf(name string) []*Widget {
	widget := s.Find(name)
	if widget == nil {
		return nil
	}
	return widget.Children
}
