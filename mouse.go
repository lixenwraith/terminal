package terminal

// MouseButton represents mouse button identity
type MouseButton uint8

const (
	MouseBtnNone MouseButton = iota
	MouseBtnLeft
	MouseBtnMiddle
	MouseBtnRight
	MouseBtnWheelUp
	MouseBtnWheelDown
	MouseBtnBack    // Button 4 (if supported)
	MouseBtnForward // Button 5 (if supported)
)

// MouseAction represents the type of mouse event
type MouseAction uint8

const (
	MouseActionNone MouseAction = iota
	MouseActionPress
	MouseActionRelease
	MouseActionMove
	MouseActionDrag
)

// MouseMode controls which mouse events are reported (bitmask)
type MouseMode uint8

const (
	MouseModeNone   MouseMode = 0
	MouseModeClick  MouseMode = 1 << 0 // Press/release events
	MouseModeDrag   MouseMode = 1 << 1 // Drag events (button held + motion)
	MouseModeMotion MouseMode = 1 << 2 // All motion events
)

// String returns human-readable button name
func (b MouseButton) String() string {
	switch b {
	case MouseBtnLeft:
		return "Left"
	case MouseBtnMiddle:
		return "Middle"
	case MouseBtnRight:
		return "Right"
	case MouseBtnWheelUp:
		return "WheelUp"
	case MouseBtnWheelDown:
		return "WheelDown"
	case MouseBtnBack:
		return "Back"
	case MouseBtnForward:
		return "Forward"
	default:
		return "None"
	}
}

// String returns human-readable action name
func (a MouseAction) String() string {
	switch a {
	case MouseActionPress:
		return "Press"
	case MouseActionRelease:
		return "Release"
	case MouseActionMove:
		return "Move"
	case MouseActionDrag:
		return "Drag"
	default:
		return "None"
	}
}