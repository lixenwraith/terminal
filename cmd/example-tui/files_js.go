package main

import (
	"embed"
	"io"
	"io/fs"
	"strings"
	"syscall/js"
)

// A browser has neither a shell nor files, so fv browses its own source there,
// read-only: the calls wasm_exec.js leaves unimplemented in its fs are answered
// from here, as Node's fs would answer them
//
//go:embed *.go
var source embed.FS

func execShell(string) {}

func init() {
	jsFS, open, last := js.Global().Get("fs"), map[int]fs.File{}, 2
	isDir := map[bool]js.Func{}
	for _, dir := range []bool{false, true} {
		isDir[dir] = js.FuncOf(func(js.Value, []js.Value) any { return dir })
	}
	fail := func(cb js.Value, code string) {
		err := js.Global().Get("Error").New(code)
		err.Set("code", code)
		cb.Invoke(err)
	}
	stat := func(cb js.Value, fi fs.FileInfo, err error) {
		if err != nil {
			fail(cb, "ENOENT")
			return
		}
		st := map[string]any{"mode": 0o100444, "size": fi.Size(), "nlink": 1, "isDirectory": isDir[fi.IsDir()]}
		if fi.IsDir() {
			st["mode"] = 0o040555
		}
		for _, k := range []string{"dev", "ino", "uid", "gid", "rdev", "blksize", "blocks", "atimeMs", "mtimeMs", "ctimeMs"} {
			st[k] = 0
		}
		cb.Invoke(js.Null(), st)
	}
	// set answers one fs call: its first argument as the path's name in source
	// and as a descriptor's file, its last the callback
	set := func(call string, fn func(name string, f fs.File, args []js.Value, cb js.Value)) {
		jsFS.Set(call, js.FuncOf(func(_ js.Value, args []js.Value) any {
			name, f := ".", fs.File(nil)
			if args[0].Type() == js.TypeNumber {
				f = open[args[0].Int()]
			} else if n := strings.Trim(args[0].String(), "/"); n != "" {
				name = n
			}
			fn(name, f, args, args[len(args)-1])
			return nil
		}))
	}
	byName := func(name string, _ fs.File, _ []js.Value, cb js.Value) {
		fi, err := fs.Stat(source, name)
		stat(cb, fi, err)
	}
	set("stat", byName)
	set("lstat", byName)
	set("fstat", func(_ string, f fs.File, _ []js.Value, cb js.Value) {
		if f == nil {
			fail(cb, "EBADF")
			return
		}
		fi, err := f.Stat()
		stat(cb, fi, err)
	})
	set("open", func(name string, _ fs.File, _ []js.Value, cb js.Value) {
		f, err := source.Open(name)
		if err != nil {
			fail(cb, "ENOENT")
			return
		}
		last++
		open[last] = f
		cb.Invoke(js.Null(), last)
	})
	set("close", func(_ string, _ fs.File, args []js.Value, cb js.Value) {
		delete(open, args[0].Int())
		cb.Invoke(js.Null())
	})
	set("readdir", func(name string, _ fs.File, _ []js.Value, cb js.Value) {
		des, _ := source.ReadDir(name)
		names := make([]any, len(des))
		for i, de := range des {
			names[i] = de.Name()
		}
		cb.Invoke(js.Null(), names)
	})
	set("read", func(_ string, f fs.File, args []js.Value, cb js.Value) {
		at, ok := f.(io.ReaderAt)
		if !ok {
			fail(cb, "EBADF")
			return
		}
		b, n := make([]byte, args[3].Int()), 0
		if args[4].IsNull() {
			n, _ = f.Read(b)
		} else {
			n, _ = at.ReadAt(b, int64(args[4].Int()))
		}
		js.CopyBytesToJS(args[1], b[:n])
		cb.Invoke(js.Null(), n)
	})
}
