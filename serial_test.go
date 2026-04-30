package serial

import (
	"reflect"
	"testing"
)

func TestModeFromString(t *testing.T) {
	good_cases := map[string]*Mode{
		"8N1": {DataBits: 8, Parity: NoParity, StopBits: OneStopBit},
		"7S2": {DataBits: 7, Parity: SpaceParity, StopBits: TwoStopBits},
	}

	bad_cases := map[string]*PortError{
		"9N1": {code: InvalidDataBits},
		"8N3": {code: InvalidStopBits},
		"8R1": {code: InvalidParity},
	}

	for s, m := range good_cases {
		mode := &Mode{}
		err := ModeFromString(s, mode)
		if err != nil {
			t.Errorf("Failed to convert mode %q: %s", s, err)
		} else if !reflect.DeepEqual(mode, m) {
			t.Errorf("Mode %q should convert to %+v, got %+v", s, m, mode)
		}
	}

	for s, e := range bad_cases {
		mode := &Mode{}
		err := ModeFromString(s, mode)
		if err == nil {
			t.Errorf("Mode %q should be invalid, got %v", s, mode)
		} else if pe, ok := err.(*PortError); !ok || pe.code != e.code {
			t.Errorf("Mode %q should fail with %v, got %v", s, e, err)
		}
	}
}
