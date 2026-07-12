package terminal

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
)

// TerminalService manages terminal lifecycle and input polling
type TerminalService struct {
	term      Terminal
	colorMode ColorMode
	eventCh   chan Event
	stopCh    chan struct{}
	doneCh    chan struct{}
	mu        sync.Mutex
	running   bool
}

// NewService creates a new terminal service
func NewService() *TerminalService {
	return &TerminalService{
		eventCh: make(chan Event, 256),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Name implements Service
func (s *TerminalService) Name() string {
	return "terminal"
}

// Dependencies implements Service
func (s *TerminalService) Dependencies() []string {
	return nil
}

// Init implements Service
// args[0]: ColorMode (optional, defaults to DetectColorMode())
func (s *TerminalService) Init(args ...any) error {
	s.colorMode = DetectColorMode()
	if len(args) > 0 {
		if cm, ok := args[0].(ColorMode); ok {
			s.colorMode = cm
		}
	}

	s.term = New(s.colorMode)
	if err := s.term.Init(); err != nil {
		return fmt.Errorf("terminal init: %w", err)
	}

	// Enable mouse click reporting
	if err := s.term.SetMouseMode(MouseModeClick); err != nil {
		// No-op for mouse error, continue
	}

	return nil
}

// Start implements Service - launches input polling goroutine
func (s *TerminalService) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	go s.pollLoop()
	return nil
}

// pollLoop reads input events until stop signal
func (s *TerminalService) pollLoop() {
	defer close(s.doneCh)

	defer func() {
		if r := recover(); r != nil {
			EmergencyReset(os.Stdout)
			os.Stdout.Sync()
			os.Stderr.Sync()
			fmt.Fprintf(os.Stderr, "\r\n\x1b[31mTERMINAL POLL CRASHED: %v\x1b[0m\r\n", r)
			fmt.Fprintf(os.Stderr, "Stack Trace:\r\n%s\r\n", debug.Stack())
			os.Stderr.Sync()
			os.Exit(1)
		}
	}()

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		ev := s.term.PollEvent()
		if ev.Type == EventClosed || ev.Type == EventError {
			return
		}

		select {
		case s.eventCh <- ev:
		case <-s.stopCh:
			return
		}
	}
}

// Stop implements Service - signals stop and restores terminal
func (s *TerminalService) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)

	// Post synthetic close event to unblock PollEvent
	if s.term != nil {
		s.term.PostEvent(Event{Type: EventClosed})
	}

	<-s.doneCh

	if s.term != nil {
		s.term.Fini()
	}
	return nil
}

// Terminal returns the wrapped terminal instance
func (s *TerminalService) Terminal() Terminal {
	return s.term
}

// Events returns the input event channel
func (s *TerminalService) Events() <-chan Event {
	return s.eventCh
}