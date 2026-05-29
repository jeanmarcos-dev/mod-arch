package programs

import "testing"

// TestCategoriesAreCoherent walks every demo and asserts:
//
//   - Category is non-empty and matches a slug declared in Categories().
//   - Difficulty is one of the three accepted values.
//
// Together these checks pin the catalogue's contract: the picker can render
// every demo under a known section with a known badge color, without falling
// back to an "Other" bucket.
func TestCategoriesAreCoherent(t *testing.T) {
	knownSlugs := map[string]bool{}
	for _, cm := range Categories() {
		knownSlugs[cm.Slug] = true
	}

	validDifficulty := map[Difficulty]bool{
		DifficultyBasic:        true,
		DifficultyIntermediate: true,
		DifficultyAdvanced:     true,
	}

	for _, d := range Demos() {
		t.Run(d.Name, func(t *testing.T) {
			if d.Category == "" {
				t.Errorf("Category is empty (expected one of the slugs in Categories())")
			} else if !knownSlugs[d.Category] {
				t.Errorf("Category %q is not declared in Categories()", d.Category)
			}
			if d.Difficulty == "" {
				t.Errorf("Difficulty is empty (expected basic|intermediate|advanced)")
			} else if !validDifficulty[d.Difficulty] {
				t.Errorf("Difficulty %q is not one of basic|intermediate|advanced", d.Difficulty)
			}
		})
	}
}

// TestCategoriesOrderIsMonotonic guards against typos in the hand-curated
// Order field of Categories(). The picker uses the field to render sections
// in canonical order; duplicates or zero values would scramble the layout.
func TestCategoriesOrderIsMonotonic(t *testing.T) {
	cats := Categories()
	for i, cm := range cats {
		if cm.Slug == "" {
			t.Errorf("Category[%d]: Slug is empty", i)
		}
		if cm.Label == "" {
			t.Errorf("Category[%s]: Label is empty", cm.Slug)
		}
		if cm.Order != i+1 {
			t.Errorf("Category[%s]: Order=%d, want %d (must match position+1)", cm.Slug, cm.Order, i+1)
		}
	}
}
