package shape

import (
	"strings"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

func TestBuildStampsTheDeclaredTypeName(t *testing.T) {
	s := Fixed("System.IO.FileInfo", Prop("Name", String, "base name"))
	got := s.Build(map[string]any{"Name": "go.mod"})

	if got[psobject.PSTypeNameKey] != "System.IO.FileInfo" {
		t.Errorf("PSTypeName = %v, want System.IO.FileInfo", got[psobject.PSTypeNameKey])
	}
}

// TestBuildOverwritesAnInferredTypeName pins the reason Build does not defer to
// a type name that is already present.
//
// psobject.NewPSObject infers one from the value it wraps, so a FileInfo built
// from a path string arrives at Build already calling itself a System.String.
// Deferring would stamp the value's type instead of the cmdlet's, which is the
// bug that shipped the first time this was written.
func TestBuildOverwritesAnInferredTypeName(t *testing.T) {
	psobj := psobject.NewPSObject("some/path")
	psobj.AddNoteProperty("Name", "path")
	if inferred := psobj.ToMap()[psobject.PSTypeNameKey]; inferred != "System.String" {
		t.Fatalf("precondition: NewPSObject inferred %v, expected System.String", inferred)
	}

	s := Fixed("System.IO.FileInfo",
		Prop("Name", String, "base name"),
		OptProp("PSPath", String, "the path"),
	)
	got := s.Build(psobj.ToMap())

	if got[psobject.PSTypeNameKey] != "System.IO.FileInfo" {
		t.Errorf("PSTypeName = %v, want the declared type to win", got[psobject.PSTypeNameKey])
	}
}

func TestBuildRecordsAMissingProperty(t *testing.T) {
	ResetDiscrepancies()
	s := Fixed("Test.Type",
		Prop("Present", String, ""),
		Prop("Absent", String, ""),
		OptProp("Optional", String, ""),
	)
	s.Build(map[string]any{"Present": "here"})

	found := Discrepancies()
	if len(found) != 1 {
		t.Fatalf("recorded %d discrepancies, want 1: %v", len(found), found)
	}
	if found[0].Property != "Absent" || found[0].Reason != ReasonMissing {
		t.Errorf("recorded %v, want Absent %s", found[0], ReasonMissing)
	}
}

func TestBuildRecordsAnUndeclaredKey(t *testing.T) {
	ResetDiscrepancies()
	Fixed("Test.Type", Prop("Known", String, "")).
		Build(map[string]any{"Known": 1, "Surprise": 2})

	found := Discrepancies()
	if len(found) != 1 {
		t.Fatalf("recorded %d discrepancies, want 1: %v", len(found), found)
	}
	if found[0].Property != "Surprise" || found[0].Reason != ReasonUndeclared {
		t.Errorf("recorded %v, want Surprise %s", found[0], ReasonUndeclared)
	}
}

// TestDerivedShapesDoNotReportUndeclaredKeys is the difference between the
// kinds. A Derived shape's extra keys came from the caller's data, so reporting
// them would fire on every correct call.
func TestDerivedShapesDoNotReportUndeclaredKeys(t *testing.T) {
	ResetDiscrepancies()
	Derived("one key per input row").
		Build(map[string]any{"whatever": 1, "the": 2, "data": 3, "held": 4})

	if found := Discrepancies(); len(found) > 0 {
		t.Errorf("a Derived shape recorded %v; its keys are the input's", found)
	}
}

// TestBuildNeverFailsTheCaller pins the bargain the recording rests on: a
// declaration that has fallen behind must degrade the catalogue, never break a
// query that was working.
func TestBuildNeverFailsTheCaller(t *testing.T) {
	ResetDiscrepancies()
	in := map[string]any{"Unexpected": "value"}
	got := Fixed("Test.Type", Prop("Missing", String, "")).Build(in)

	if got["Unexpected"] != "value" {
		t.Error("Build dropped a key it did not recognise; it must pass the object through")
	}
	if len(Discrepancies()) == 0 {
		t.Error("Build passed the object through without recording the disagreement")
	}
}

func TestUnspecifiedShapeDescribesItselfAsNothing(t *testing.T) {
	var nilShape *Shape
	for _, s := range []*Shape{nilShape, Scalar()} {
		if s.Specified() {
			t.Error("an undeclared shape reports itself as specified")
		}
		if s.Describe() != "" || s.Compact() != "" || s.Summary() != "" {
			t.Errorf("an undeclared shape described itself as %q", s.Describe())
		}
	}
	// A nil shape is what ShapeOf returns for the ~300 cmdlets that declare
	// none, so every method has to tolerate it rather than panic.
	if nilShape.TypeName() != "" || nilShape.Properties() != nil {
		t.Error("a nil shape did not behave as an empty one")
	}
}

func TestPlainShapeOmitsATypeName(t *testing.T) {
	s := Plain(Prop("count", Number, "how many"))
	if got := s.Build(map[string]any{"count": 1}); got[psobject.PSTypeNameKey] != nil {
		t.Errorf("a Plain shape stamped %v; it must not add a type name", got[psobject.PSTypeNameKey])
	}
	if !strings.HasPrefix(s.Summary(), "object with 1 property") {
		t.Errorf("Summary = %q", s.Summary())
	}
}

func TestNoteAppearsInTheDescription(t *testing.T) {
	s := Plain(Prop("a", String, "")).Note("returns a string when given a Format option")
	if !strings.Contains(s.Describe(), "Format option") {
		t.Errorf("Describe dropped the note: %q", s.Describe())
	}
	if !strings.Contains(s.Compact(), "Format option") {
		t.Errorf("Compact dropped the note: %q", s.Compact())
	}
}

func TestDescribeMarksOptionalProperties(t *testing.T) {
	s := Fixed("Test.Type",
		Prop("Always", String, "always here"),
		OptProp("Sometimes", Number, "only sometimes"),
	)
	described := s.Describe()
	if !strings.Contains(described, "Sometimes (number, optional)") {
		t.Errorf("Describe did not mark the optional property:\n%s", described)
	}
	if strings.Contains(described, "Always (string, optional)") {
		t.Errorf("Describe marked a required property optional:\n%s", described)
	}
	if !strings.Contains(s.Compact(), "Sometimes?") {
		t.Errorf("Compact did not mark the optional property: %q", s.Compact())
	}
}
