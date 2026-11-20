//go:build linux || freebsd

package window

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The shared objects a Mesa or vendor desktop installs under the same names on
// Linux and FreeBSD; the unversioned names belong to the development packages.
const (
	libX11    = "libX11.so.6"
	libEGL    = "libEGL.so.1"
	libGLESv2 = "libGLESv2.so.2"
)

// Xlib, EGL and GLES 2 entry points. C int is int32 here: a 32-bit return leaves
// the upper half of its register undefined.
var (
	xOpenDisplay        func(name uintptr) uintptr
	xCloseDisplay       func(dpy uintptr) int32
	xDefaultScreen      func(dpy uintptr) int32
	xRootWindow         func(dpy uintptr, screen int32) uintptr
	xBlackPixel         func(dpy uintptr, screen int32) uint64
	xCreateSimpleWindow func(dpy, parent uintptr, x, y int32, w, h, border uint32, borderPixel, background uint64) uintptr
	xDestroyWindow      func(dpy, win uintptr) int32
	xSelectInput        func(dpy, win uintptr, mask int64) int32
	xStoreName          func(dpy, win uintptr, name string) int32
	xInternAtom         func(dpy uintptr, name string, onlyIfExists int32) uintptr
	xSetWMProtocols     func(dpy, win uintptr, atoms *uintptr, count int32) int32
	xSetWMNormalHints   func(dpy, win uintptr, hints *sizeHints)
	xChangeProperty     func(dpy, win, property, typ uintptr, format, mode int32, data *uintptr, n int32) int32
	xMapWindow          func(dpy, win uintptr) int32
	xConnectionNumber   func(dpy uintptr) int32
	xPending            func(dpy uintptr) int32
	xNextEvent          func(dpy uintptr, ev *xEvent) int32
	xLookupString       func(ev *xEvent, buf *byte, size int32, keysym *uint64, status uintptr) int32
	xFlush              func(dpy uintptr) int32

	xkbSetDetectableAutoRepeat func(dpy uintptr, detectable int32, supported *int32) int32

	eglGetDisplay          func(native uintptr) uintptr
	eglGetPlatformDisplay  func(platform uint32, native uintptr, attribs uintptr) uintptr
	eglInitialize          func(dpy uintptr, major, minor *int32) uint32
	eglBindAPI             func(api uint32) uint32
	eglChooseConfig        func(dpy uintptr, attribs []int32, configs *uintptr, size int32, n *int32) uint32
	eglCreateWindowSurface func(dpy, config, win uintptr, attribs []int32) uintptr
	eglCreateContext       func(dpy, config, share uintptr, attribs []int32) uintptr
	eglMakeCurrent         func(dpy, draw, read, ctx uintptr) uint32
	eglSwapBuffers         func(dpy, surface uintptr) uint32
	eglSwapInterval        func(dpy uintptr, interval int32) uint32
	eglDestroySurface      func(dpy, surface uintptr) uint32
	eglDestroyContext      func(dpy, ctx uintptr) uint32
	eglTerminate           func(dpy uintptr) uint32
	eglGetError            func() int32

	glCreateShader            func(kind uint32) uint32
	glShaderSource            func(shader uint32, count int32, src **byte, length *int32)
	glCompileShader           func(shader uint32)
	glGetShaderiv             func(shader, pname uint32, out *int32)
	glGetShaderInfoLog        func(shader uint32, size int32, length *int32, log *byte)
	glCreateProgram           func() uint32
	glAttachShader            func(program, shader uint32)
	glBindAttribLocation      func(program, index uint32, name string)
	glLinkProgram             func(program uint32)
	glGetProgramiv            func(program, pname uint32, out *int32)
	glUseProgram              func(program uint32)
	glGetUniformLocation      func(program uint32, name string) int32
	glUniform1i               func(location, v int32)
	glGenTextures             func(n int32, out *uint32)
	glBindTexture             func(target, texture uint32)
	glTexParameteri           func(target, pname uint32, param int32)
	glPixelStorei             func(pname uint32, param int32)
	glTexImage2D              func(target uint32, level, internal, w, h, border int32, format, kind uint32, pix unsafe.Pointer)
	glTexSubImage2D           func(target uint32, level, x, y, w, h int32, format, kind uint32, pix unsafe.Pointer)
	glGenBuffers              func(n int32, out *uint32)
	glBindBuffer              func(target, buffer uint32)
	glBufferData              func(target uint32, size int, data []float32, usage uint32)
	glVertexAttribPointer     func(index uint32, size int32, kind uint32, normalized uint8, stride int32, offset uintptr)
	glEnableVertexAttribArray func(index uint32)
	glViewport                func(x, y, w, h int32)
	glClearColor              func(r, g, b, a float32)
	glClear                   func(mask uint32)
	glDrawArrays              func(mode uint32, first, count int32)
	glFinish                  func()
)

const (
	eglNone, eglSurfaceType, eglWindowBit         = 0x3038, 0x3033, 0x0004
	eglRenderableType, eglOpenGLES2Bit            = 0x3040, 0x0004
	eglRedSize, eglGreenSize, eglBlueSize         = 0x3024, 0x3023, 0x3022
	eglContextClientVersion, eglOpenGLESAPI       = 0x3098, 0x30A0
	eglPlatformX11                                = 0x31D5
	glVertexShader, glFragmentShader              = 0x8B31, 0x8B30
	glCompileStatus, glLinkStatus, glInfoLogLen   = 0x8B81, 0x8B82, 0x8B84
	glTexture2D, glTextureMinFilter, glTextureMag = 0x0DE1, 0x2801, 0x2800
	glTextureWrapS, glTextureWrapT                = 0x2802, 0x2803
	glNearest, glLinear, glClampToEdge            = 0x2600, 0x2601, 0x812F
	glRGBA, glUnsignedByte, glFloat               = 0x1908, 0x1401, 0x1406
	glUnpackAlignment                             = 0x0CF5
	glArrayBuffer, glStaticDraw                   = 0x8892, 0x88E4
	glColorBufferBit, glTriangleStrip             = 0x4000, 0x0005
)

type symbol struct {
	name string
	fptr any
}

var loaded struct {
	done bool
	err  error
}

// load opens the three libraries and binds every entry point, once; a missing
// library or symbol is an error, never a panic
func load() error {
	if loaded.done {
		return loaded.err
	}
	loaded.done = true
	libs := []struct {
		path     string
		syms     []symbol
		optional []symbol // left nil when the library lacks them
	}{
		{libX11, []symbol{
			{"XOpenDisplay", &xOpenDisplay}, {"XCloseDisplay", &xCloseDisplay},
			{"XDefaultScreen", &xDefaultScreen}, {"XRootWindow", &xRootWindow},
			{"XBlackPixel", &xBlackPixel}, {"XCreateSimpleWindow", &xCreateSimpleWindow},
			{"XDestroyWindow", &xDestroyWindow}, {"XSelectInput", &xSelectInput},
			{"XStoreName", &xStoreName}, {"XInternAtom", &xInternAtom},
			{"XSetWMProtocols", &xSetWMProtocols}, {"XSetWMNormalHints", &xSetWMNormalHints},
			{"XChangeProperty", &xChangeProperty},
			{"XMapWindow", &xMapWindow}, {"XConnectionNumber", &xConnectionNumber},
			{"XPending", &xPending}, {"XNextEvent", &xNextEvent},
			{"XLookupString", &xLookupString}, {"XFlush", &xFlush},
		}, []symbol{{"XkbSetDetectableAutoRepeat", &xkbSetDetectableAutoRepeat}}},
		{libEGL, []symbol{
			{"eglGetDisplay", &eglGetDisplay}, {"eglInitialize", &eglInitialize},
			{"eglBindAPI", &eglBindAPI}, {"eglChooseConfig", &eglChooseConfig},
			{"eglCreateWindowSurface", &eglCreateWindowSurface}, {"eglCreateContext", &eglCreateContext},
			{"eglMakeCurrent", &eglMakeCurrent}, {"eglSwapBuffers", &eglSwapBuffers},
			{"eglSwapInterval", &eglSwapInterval}, {"eglDestroySurface", &eglDestroySurface},
			{"eglDestroyContext", &eglDestroyContext}, {"eglTerminate", &eglTerminate},
			{"eglGetError", &eglGetError},
		}, []symbol{{"eglGetPlatformDisplay", &eglGetPlatformDisplay}}},
		{libGLESv2, []symbol{
			{"glCreateShader", &glCreateShader}, {"glShaderSource", &glShaderSource},
			{"glCompileShader", &glCompileShader}, {"glGetShaderiv", &glGetShaderiv},
			{"glGetShaderInfoLog", &glGetShaderInfoLog}, {"glCreateProgram", &glCreateProgram},
			{"glAttachShader", &glAttachShader}, {"glBindAttribLocation", &glBindAttribLocation},
			{"glLinkProgram", &glLinkProgram}, {"glGetProgramiv", &glGetProgramiv},
			{"glUseProgram", &glUseProgram}, {"glGetUniformLocation", &glGetUniformLocation},
			{"glUniform1i", &glUniform1i}, {"glGenTextures", &glGenTextures},
			{"glBindTexture", &glBindTexture}, {"glTexParameteri", &glTexParameteri},
			{"glPixelStorei", &glPixelStorei}, {"glTexImage2D", &glTexImage2D},
			{"glTexSubImage2D", &glTexSubImage2D}, {"glGenBuffers", &glGenBuffers},
			{"glBindBuffer", &glBindBuffer}, {"glBufferData", &glBufferData},
			{"glVertexAttribPointer", &glVertexAttribPointer},
			{"glEnableVertexAttribArray", &glEnableVertexAttribArray},
			{"glViewport", &glViewport}, {"glClearColor", &glClearColor}, {"glClear", &glClear},
			{"glDrawArrays", &glDrawArrays}, {"glFinish", &glFinish},
		}, nil},
	}
	for _, lib := range libs {
		h, err := purego.Dlopen(lib.path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loaded.err = fmt.Errorf("window: %w", err) // dlerror names the library
			return loaded.err
		}
		for _, s := range lib.syms {
			fn, err := purego.Dlsym(h, s.name)
			if err != nil {
				loaded.err = fmt.Errorf("window: %s: %s: %w", lib.path, s.name, err)
				return loaded.err
			}
			purego.RegisterFunc(s.fptr, fn)
		}
		for _, s := range lib.optional {
			if fn, err := purego.Dlsym(h, s.name); err == nil {
				purego.RegisterFunc(s.fptr, fn)
			}
		}
	}
	return nil
}

// gpu is an EGL context on the window and the one textured quad a frame is drawn
// as, a cell of its texels over a cell of window pixels
type gpu struct {
	display, surface, context uintptr
	texture                   uint32
	texW, texH                int // the texture's allocated size, 0 before the first frame
	cw, ch, tw, th            int // a cell's window pixels and its texels
}

// The quad covers the viewport; the texture's first row is the frame's top row
const (
	vertexSource = `attribute vec2 p;
varying vec2 uv;
void main() { uv = vec2(p.x + 1.0, 1.0 - p.y) * 0.5; gl_Position = vec4(p, 0.0, 1.0); }` + "\x00"
	fragmentSource = `precision mediump float;
varying vec2 uv;
uniform sampler2D frame;
void main() { gl_FragColor = texture2D(frame, uv); }` + "\x00"
)

var quad = []float32{-1, -1, 1, -1, -1, 1, 1, 1}

// newGPU makes a GLES 2 context current on the calling thread, drawing to win
func newGPU(xdpy, win uintptr) (*gpu, error) {
	g := &gpu{cw: 1, ch: 1, tw: 1, th: 1}
	// EGL 1.5 names the platform; an older one guesses it from the pointer
	if eglGetPlatformDisplay != nil {
		g.display = eglGetPlatformDisplay(eglPlatformX11, xdpy, 0)
	} else {
		g.display = eglGetDisplay(xdpy)
	}
	if g.display == 0 || eglInitialize(g.display, nil, nil) == 0 {
		return nil, eglErr("eglInitialize")
	}
	if eglBindAPI(eglOpenGLESAPI) == 0 {
		g.release()
		return nil, eglErr("eglBindAPI")
	}
	var config uintptr
	var n int32
	attribs := []int32{eglSurfaceType, eglWindowBit, eglRenderableType, eglOpenGLES2Bit,
		eglRedSize, 8, eglGreenSize, 8, eglBlueSize, 8, eglNone}
	if eglChooseConfig(g.display, attribs, &config, 1, &n) == 0 || n == 0 {
		g.release()
		return nil, errors.New("window: no EGL config draws GLES 2 to a window")
	}
	if g.surface = eglCreateWindowSurface(g.display, config, win, nil); g.surface == 0 {
		g.release()
		return nil, eglErr("eglCreateWindowSurface")
	}
	if g.context = eglCreateContext(g.display, config, 0, []int32{eglContextClientVersion, 2, eglNone}); g.context == 0 {
		g.release()
		return nil, eglErr("eglCreateContext")
	}
	if eglMakeCurrent(g.display, g.surface, g.surface, g.context) == 0 {
		g.release()
		return nil, eglErr("eglMakeCurrent")
	}
	eglSwapInterval(g.display, 1)
	if err := g.setup(); err != nil {
		g.release()
		return nil, err
	}
	return g, nil
}

func (g *gpu) setup() error {
	program := glCreateProgram()
	for _, s := range []struct {
		kind uint32
		src  string
	}{{glVertexShader, vertexSource}, {glFragmentShader, fragmentSource}} {
		shader, err := compile(s.kind, s.src)
		if err != nil {
			return err
		}
		glAttachShader(program, shader)
	}
	glBindAttribLocation(program, 0, "p\x00")
	glLinkProgram(program)
	var ok int32
	if glGetProgramiv(program, glLinkStatus, &ok); ok == 0 {
		return errors.New("window: the frame shader does not link")
	}
	glUseProgram(program)
	glUniform1i(glGetUniformLocation(program, "frame\x00"), 0)

	var vbo uint32
	glGenBuffers(1, &vbo)
	glBindBuffer(glArrayBuffer, vbo)
	glBufferData(glArrayBuffer, len(quad)*4, quad, glStaticDraw)
	glVertexAttribPointer(0, 2, glFloat, 0, 0, 0)
	glEnableVertexAttribArray(0)

	glGenTextures(1, &g.texture)
	glBindTexture(glTexture2D, g.texture)
	for _, p := range [][2]uint32{{glTextureMinFilter, glNearest}, {glTextureMag, glNearest},
		{glTextureWrapS, glClampToEdge}, {glTextureWrapT, glClampToEdge}} {
		glTexParameteri(glTexture2D, p[0], int32(p[1]))
	}
	glPixelStorei(glUnpackAlignment, 1)
	glClearColor(0, 0, 0, 1)
	return nil
}

func compile(kind uint32, src string) (uint32, error) {
	shader := glCreateShader(kind)
	buf := []byte(src)
	ptr := []*byte{&buf[0]}
	var pin runtime.Pinner
	pin.Pin(&buf[0])
	pin.Pin(&ptr[0])
	glShaderSource(shader, 1, &ptr[0], nil)
	pin.Unpin()
	glCompileShader(shader)
	var ok int32
	if glGetShaderiv(shader, glCompileStatus, &ok); ok != 0 {
		return shader, nil
	}
	var size int32
	glGetShaderiv(shader, glInfoLogLen, &size)
	log := make([]byte, max(size, 1))
	glGetShaderInfoLog(shader, int32(len(log)), nil, &log[0])
	return 0, fmt.Errorf("window: shader: %s", log)
}

// upload replaces the texture with an RGBA frame, allocating it when the size changed
func (g *gpu) upload(pix []byte, width, height int) {
	if width == 0 || height == 0 || len(pix) < width*height*4 {
		return
	}
	if width != g.texW || height != g.texH {
		g.texW, g.texH = width, height
		glTexImage2D(glTexture2D, 0, glRGBA, int32(width), int32(height), 0, glRGBA, glUnsignedByte, unsafe.Pointer(&pix[0]))
		return
	}
	glTexSubImage2D(glTexture2D, 0, 0, 0, int32(width), int32(height), glRGBA, glUnsignedByte, unsafe.Pointer(&pix[0]))
}

// cell sets the window pixels a cell of tw by th texels is drawn over: a whole
// multiple keeps each texel a block of whole pixels, and any other is filtered
func (g *gpu) cell(cw, ch, tw, th int) {
	g.cw, g.ch, g.tw, g.th = cw, ch, tw, th
	filter := int32(glLinear)
	if cw%tw == 0 && ch%th == 0 {
		filter = glNearest
	}
	glTexParameteri(glTexture2D, glTextureMinFilter, filter)
	glTexParameteri(glTexture2D, glTextureMag, filter)
}

// draw clears the window and draws the frame at its cell's size, centred
func (g *gpu) draw(winW, winH int) {
	glViewport(0, 0, int32(winW), int32(winH))
	glClear(glColorBufferBit)
	if g.texW > 0 {
		w, h := g.texW/g.tw*g.cw, g.texH/g.th*g.ch
		x, y := (winW-w)/2, (winH-h)/2
		glViewport(int32(x), int32(winH-y-h), int32(w), int32(h))
		glDrawArrays(glTriangleStrip, 0, 4)
	}
	eglSwapBuffers(g.display, g.surface)
}

func (g *gpu) release() {
	if g.display == 0 {
		return
	}
	eglMakeCurrent(g.display, 0, 0, 0)
	if g.context != 0 {
		eglDestroyContext(g.display, g.context)
	}
	if g.surface != 0 {
		eglDestroySurface(g.display, g.surface)
	}
	eglTerminate(g.display)
	g.display = 0
}

func eglErr(call string) error {
	return fmt.Errorf("window: %s failed, EGL error 0x%x", call, eglGetError())
}
