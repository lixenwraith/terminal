/* TerminalPage is the page a program built on lixenwraith/terminal runs in: it
   draws the frames the program's Worker (worker.js) hands it and reports keys,
   text, the pointer and its size back. README.md beside this file is its contract. */
(function() {
    'use strict';

    /* A touch-first screen (phone, tablet), decided once at load, keeps the system
       keyboard down on taps: the page's own script or three taps on the bar raise
       it. A cell is CELL CSS pixels tall and half as wide until open, the program or
       a zoom names another: a pinch, or Ctrl and the wheel, within the program's
       range, ZOOM where it names none. */
    const touch = matchMedia('(pointer: coarse)').matches;
    const CELL = 16;
    const ZOOM = { least: 4, most: 40, step: 0.5 };
    const FONT = '"JetBrains Mono", "Fira Code", "SF Mono", Menlo, "DejaVu Sans Mono", Consolas, monospace';
    const FACE = 'TerminalPage';   // the family a program's own font is loaded as
    const CALLBACKS = ['progress', 'compiling', 'ready', 'frame', 'message', 'keep', 'gesture',
        'keyboard', 'notice', 'exit', 'failed'];

    let container, canvas, input, strip;   // open's elements; the key strip may be null
    const on = {};          // open's callbacks, each a function
    let stack = FONT;       // the page's own fonts
    let font = FONT;        // what glyphs are drawn in: the program's face, if any, then stack
    let cellBase = CELL;    // the cell the grid is fitted at
    let program;            // the Worker running Go, see worker.js; null once stopped
    let drawer;             // the screen, below
    let cols = 0, rows = 0; // the grid the page reported to Go

    // Posts an event to Go; before the program starts, the start message carries the grid.
    function send(ev) {
        if (program) program.postMessage({ type: 'event', event: ev });
    }

    // tell calls back once open has returned its page, which the callback may use
    function tell(name, ...args) {
        Promise.resolve().then(function() { on[name](...args); });
    }

    // === The screen ===
    /* One WebGL quad over the frame. A fragment finds its cell in two data textures,
       the cell's colors and glyph slot, and mixes the colors by the glyph's mask,
       rasterized at the cell's device size, so every glyph is drawn pixel for pixel
       at any zoom. A frame arrives as the cells changed since the last one
       (terminal.RuneFrame): index, rune, foreground, background. */
    const VERTEX = `
attribute vec2 a_pos;
uniform vec4 u_quad;
varying vec2 v_uv;
void main() {
    v_uv = a_pos;
    gl_Position = vec4(u_quad.xy + a_pos * u_quad.zw, 0.0, 1.0);
}`;
    const FRAGMENT = `
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif
uniform sampler2D u_fg;
uniform sampler2D u_bg;
uniform sampler2D u_mask;
uniform vec2 u_grid;
uniform vec4 u_src;
uniform vec2 u_slots;
varying vec2 v_uv;
void main() {
    vec2 pos = u_src.xy + v_uv * u_src.zw;
    vec2 cuv = (floor(pos) + 0.5) / u_grid;
    vec4 fg = texture2D(u_fg, cuv);
    vec4 bg = texture2D(u_bg, cuv);
    float slot = floor(fg.a * 255.0 + 0.5) + floor(bg.a * 255.0 + 0.5) * 256.0 - 1.0;
    float m = 0.0;
    if (slot >= 0.0) {
        if (slot >= u_slots.x * u_slots.y) slot = 0.0;
        vec2 at = vec2(mod(slot, u_slots.x), floor(slot / u_slots.x));
        m = texture2D(u_mask, (at + fract(pos)) / u_slots).a;
    }
    gl_FragColor = vec4(mix(bg.rgb, fg.rgb, m), 1.0);
}`;

    /* The glyphs are the page's: a rune takes the next slot in the first frame that
       brings it, and both drawers read its mask from one canvas of slots. Slot 0 is
       U+FFFD, which a rune the mask has no room for draws as; a blank has no slot.
       A cell keeps its slot and 1 in two bytes, hence SLOTS_MOST. */
    const SLOTS_MOST = 0xfffe;
    const MASK_MOST = 4096;   // px a side of the mask: what every browser's 2D canvas holds
    const glyphs = {
        slots: new Map([[0xfffd, 0]]),   // code point to slot
        runes: ['\ufffd'],              // by slot
        canvas: document.createElement('canvas'),
        cols: 1, rows: 0,                // the mask's slots a row, and its rows
        drawn: 0,                        // the runes it holds
        limit: MASK_MOST,                // px a side: a GPU's texture limit, where lower
    };

    // slotOf is a rune's slot, taken the first time it is asked for; -1 for a blank
    function slotOf(rune) {
        if (rune === 0 || rune === 32) return -1;
        let slot = glyphs.slots.get(rune);
        if (slot === undefined) {
            if (glyphs.runes.length >= SLOTS_MOST || rune > 0x10ffff) return 0;
            slot = glyphs.runes.length;
            glyphs.slots.set(rune, slot);
            glyphs.runes.push(String.fromCodePoint(rune));
        }
        return slot;
    }

    /* rasterizeMask draws the glyphs the mask lacks, white on transparent, at the
       cell's device size, and every one again once it grew a row or `all` says the
       cell or the font changed; it reports whether it drew. A row holds the slots
       that fit the limit, a power of two, which a shader divides by exactly; the
       rows grow to that limit too, and a slot past them draws as slot 0. */
    function rasterizeMask(cw, ch, all) {
        const n = glyphs.runes.length, c = glyphs.canvas;
        let slotCols = 1;
        while (slotCols * 2 * cw <= glyphs.limit) slotCols *= 2;
        const slotRows = Math.min(Math.max(1, Math.floor(glyphs.limit / ch)), Math.ceil(n / slotCols));
        let from = glyphs.drawn;
        if (all || slotCols !== glyphs.cols || slotRows !== glyphs.rows) {
            glyphs.cols = slotCols;
            glyphs.rows = slotRows;
            c.width = slotCols * cw;   // which clears it
            c.height = slotRows * ch;
            from = 0;
        }
        const to = Math.min(n, slotCols * slotRows);
        glyphs.drawn = n;
        if (from >= to) return false;
        const ctx = c.getContext('2d');
        drawGlyphs(ctx, cw, ch, from, to);
        return true;
    }

    /* drawGlyphs draws slots from to to: the font sized so its advance is the cell's
       width, then stretched to the cell's height, so box-drawing glyphs meet their
       neighbours both ways; a glyph wider than its cell is clipped to it. Slot i is
       at column i % glyphs.cols. */
    function drawGlyphs(ctx, cw, ch, from, to) {
        const runes = glyphs.runes, slotCols = glyphs.cols;
        ctx.font = ch + 'px ' + font;
        let m = ctx.measureText('M');
        const size = Math.max(1, ch * cw / (m.width || ch));
        ctx.font = size + 'px ' + font;
        m = ctx.measureText('M');
        const asc = m.fontBoundingBoxAscent || size * 0.8;
        const desc = m.fontBoundingBoxDescent || size * 0.2;
        const sy = ch / (asc + desc);
        ctx.fillStyle = '#fff';
        ctx.textBaseline = 'alphabetic';
        for (let i = from; i < to; i++) {
            const x = (i % slotCols) * cw, y = Math.floor(i / slotCols) * ch;
            ctx.save();
            ctx.beginPath();
            ctx.rect(x, y, cw, ch);
            ctx.clip();
            ctx.setTransform(1, 0, 0, sy, 0, 0);
            ctx.fillText(runes[i], x + (cw - ctx.measureText(runes[i]).width) / 2, y / sy + asc);
            ctx.restore();
        }
    }

    // applyFrame takes a frame into a drawer's cell arrays: the cells changed since
    // the one held, or all after a resize; it returns the changed indices
    function applyFrame(d, data) {
        const w = new Uint32Array(data.buffer, data.byteOffset, data.byteLength >> 2);
        const cols = w[0], rows = w[1], n = w[2];
        if (cols !== d.cols || rows !== d.rows) {
            d.cols = cols;
            d.rows = rows;
            d.fg = new Uint8Array(cols * rows * 4);
            d.bg = new Uint8Array(cols * rows * 4);
            d.resized();
        }
        const fg = d.fg, bg = d.bg, changed = new Uint32Array(n);
        for (let i = 0, at = 3; i < n; i++, at += 4) {
            const idx = w[at], o = idx * 4, slot = slotOf(w[at + 1]) + 1, f = w[at + 2], b = w[at + 3];
            fg[o] = f >>> 16 & 255; fg[o + 1] = f >>> 8 & 255; fg[o + 2] = f & 255; fg[o + 3] = slot & 255;
            bg[o] = b >>> 16 & 255; bg[o + 1] = b >>> 8 & 255; bg[o + 2] = b & 255; bg[o + 3] = slot >>> 8 & 255;
            changed[i] = idx;
        }
        return changed;
    }

    // The frame a drawer holds in device pixels, and where it is drawn: at the top,
    // centred when narrower than the canvas
    function frameOf(d) { return [d.cols * d.cw, d.rows * d.ch]; }
    function originOf(d) { return [Math.max(0, Math.floor((canvas.width - d.cols * d.cw) / 2)), 0]; }

    /* While a keyboard covers the canvas's bottom `covered` device pixels, the last
       row, where a program keeps its input, is drawn again just above it at twice its
       size: its left half fills the frame's width, and the program, told of the
       keyboard, keeps its input to that half. Null with no keyboard. */
    let covered = 0;
    function stripOf(d) {
        if (!covered || !d.cols) return null;
        const half = Math.floor(d.cols / 2), [ox] = originOf(d), bottom = canvas.height - covered;
        return { cols: half, row: d.rows - 1, x: ox, y: bottom - 2 * d.ch, w: 2 * half * d.cw, h: 2 * d.ch };
    }

    // setCovered keeps the device pixels a keyboard covers, tells Go the grid's rows
    // it and the strip hide, and draws the strip where it now belongs; fit calls it
    // again as the grid changes under a keyboard
    let coveredRows = 0;
    function setCovered(px) {
        px = Math.max(0, px);
        const ch = drawer.ch || 1;
        const hidden = px ? Math.min(rows, Math.max(1, rows - Math.floor((canvas.height - px - 2 * ch) / ch))) : 0;
        if (hidden !== coveredRows) {
            coveredRows = hidden;
            send({ type: 'covered', rows: hidden });
        }
        if (px === covered) return;
        covered = px;
        drawer.draw();
    }

    function Drawer(canvas) {
        const gl = canvas.getContext('webgl', { alpha: false, antialias: false, depth: false, stencil: false });
        if (!gl) throw new Error('WebGL is unavailable');
        this.gl = gl;
        this.cw = 0; this.ch = 0;       // a cell in device pixels
        this.cols = 0; this.rows = 0;   // the frame held
        this.fg = null; this.bg = null; // its cells: RGB and a byte of the slot each
        this.dirty = false;
        this.lost = false;
        canvas.addEventListener('webglcontextlost', e => { e.preventDefault(); this.lost = true; });
        canvas.addEventListener('webglcontextrestored', () => {
            this.lost = false;
            this.setup();
            this.rasterize(false);
            this.dirty = true;
            this.draw();
            send({ type: 'sync' });
        });
        this.setup();
    }

    Drawer.prototype.setup = function() {
        const gl = this.gl;
        const program = gl.createProgram();
        for (const [kind, src] of [[gl.VERTEX_SHADER, VERTEX], [gl.FRAGMENT_SHADER, FRAGMENT]]) {
            const shader = gl.createShader(kind);
            gl.shaderSource(shader, src);
            gl.compileShader(shader);
            if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS) && !gl.isContextLost()) {
                throw new Error('shader: ' + gl.getShaderInfoLog(shader));
            }
            gl.attachShader(program, shader);
        }
        gl.linkProgram(program);
        if (!gl.getProgramParameter(program, gl.LINK_STATUS) && !gl.isContextLost()) {
            throw new Error('shader: ' + gl.getProgramInfoLog(program));
        }
        gl.useProgram(program);
        gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer());
        gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([0, 0, 1, 0, 0, 1, 1, 1]), gl.STATIC_DRAW);
        const pos = gl.getAttribLocation(program, 'a_pos');
        gl.enableVertexAttribArray(pos);
        gl.vertexAttribPointer(pos, 2, gl.FLOAT, false, 0, 0);
        this.u = {};
        for (const name of ['u_quad', 'u_grid', 'u_src', 'u_slots', 'u_fg', 'u_bg', 'u_mask']) {
            this.u[name] = gl.getUniformLocation(program, name);
        }
        this.tex = {};
        [['u_fg', 0], ['u_bg', 1], ['u_mask', 2]].forEach(([name, unit]) => {
            const t = gl.createTexture();
            gl.activeTexture(gl.TEXTURE0 + unit);
            gl.bindTexture(gl.TEXTURE_2D, t);
            gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
            gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
            gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
            gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
            gl.uniform1i(this.u[name], unit);
            this.tex[name] = t;
        });
        gl.clearColor(0, 0, 0, 1);
        glyphs.limit = Math.min(MASK_MOST, gl.getParameter(gl.MAX_TEXTURE_SIZE) || MASK_MOST);
        this.masked = false;   // the texture holds the mask as the canvas has it
    };

    // rasterize brings the mask to the runes known and uploads it when it changed; a
    // lost context's is uploaded as it comes back
    Drawer.prototype.rasterize = function(all) {
        if (!this.cw) return;
        const drew = rasterizeMask(this.cw, this.ch, all);
        if (this.lost || this.masked && !drew) return;
        const gl = this.gl;
        gl.activeTexture(gl.TEXTURE2);
        gl.bindTexture(gl.TEXTURE_2D, this.tex.u_mask);
        gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, glyphs.canvas);
        gl.uniform2f(this.u.u_slots, glyphs.cols, glyphs.rows);
        this.masked = true;
    };

    Drawer.prototype.resized = function() {};
    Drawer.prototype.apply = function(data) {
        applyFrame(this, data);
        this.rasterize(false);
        this.dirty = true;
    };
    Drawer.prototype.frame = function() { return frameOf(this); };
    Drawer.prototype.origin = function() { return originOf(this); };

    Drawer.prototype.draw = function() {
        const gl = this.gl;
        if (this.lost) return;
        const W = canvas.width, H = canvas.height;
        gl.viewport(0, 0, W, H);
        gl.clear(gl.COLOR_BUFFER_BIT);
        if (!this.cols || !this.masked) return;
        if (this.dirty) {
            gl.activeTexture(gl.TEXTURE0);
            gl.bindTexture(gl.TEXTURE_2D, this.tex.u_fg);
            gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, this.cols, this.rows, 0, gl.RGBA, gl.UNSIGNED_BYTE, this.fg);
            gl.activeTexture(gl.TEXTURE1);
            gl.bindTexture(gl.TEXTURE_2D, this.tex.u_bg);
            gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, this.cols, this.rows, 0, gl.RGBA, gl.UNSIGNED_BYTE, this.bg);
            gl.uniform2f(this.u.u_grid, this.cols, this.rows);
            this.dirty = false;
        }
        const [fw, fh] = this.frame(), [ox, oy] = this.origin();
        gl.uniform4f(this.u.u_quad, -1 + 2 * ox / W, 1 - 2 * oy / H, 2 * fw / W, -2 * fh / H);
        gl.uniform4f(this.u.u_src, 0, 0, this.cols, this.rows);
        gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
        const s = stripOf(this);
        if (s) {
            gl.uniform4f(this.u.u_quad, -1 + 2 * s.x / W, 1 - 2 * s.y / H, 2 * s.w / W, -2 * s.h / H);
            gl.uniform4f(this.u.u_src, 0, s.row, s.cols, 1);
            gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
        }
    };

    /* Painter is the screen without WebGL: each changed cell's background and then
       its glyph, cut from the mask and tinted through a scratch cell, painted into a
       picture the frame's size that is then drawn on the canvas. Slower on a large
       grid, which the page says once at the start. */
    function Painter(canvas) {
        this.ctx = canvas.getContext('2d', { alpha: false });
        if (!this.ctx) throw new Error('Canvas 2D is unavailable');
        this.cw = 0; this.ch = 0;
        this.cols = 0; this.rows = 0;
        this.fg = null; this.bg = null;
        this.masked = false;
        this.lost = false;
        this.picture = document.createElement('canvas');
        this.pic = this.picture.getContext('2d', { alpha: false });
        this.scratch = document.createElement('canvas');
        this.sc = this.scratch.getContext('2d');
    }

    // rasterize brings the mask to the runes known and, with all of it drawn again,
    // paints every cell again from it
    Painter.prototype.rasterize = function(all) {
        if (!this.cw) return;
        rasterizeMask(this.cw, this.ch, all);
        this.masked = true;
        if (!all) return;
        this.scratch.width = this.cw;
        this.scratch.height = this.ch;
        this.resized();
        if (this.fg) for (let i = 0; i < this.cols * this.rows; i++) this.paint(i);
    };

    Painter.prototype.resized = function() {
        const [fw, fh] = this.frame();
        this.picture.width = Math.max(1, fw);
        this.picture.height = Math.max(1, fh);
        this.pic.fillStyle = '#000';
        this.pic.fillRect(0, 0, this.picture.width, this.picture.height);
    };

    Painter.prototype.apply = function(data) {
        const changed = applyFrame(this, data);
        if (!this.masked) return;
        this.rasterize(false);
        for (let i = 0; i < changed.length; i++) this.paint(changed[i]);
    };

    Painter.prototype.paint = function(i) {
        const fg = this.fg, bg = this.bg, o = i * 4, cw = this.cw, ch = this.ch;
        const x = (i % this.cols) * cw, y = Math.floor(i / this.cols) * ch;
        const pic = this.pic;
        pic.fillStyle = 'rgb(' + bg[o] + ',' + bg[o + 1] + ',' + bg[o + 2] + ')';
        pic.fillRect(x, y, cw, ch);
        let slot = fg[o + 3] + bg[o + 3] * 256 - 1;
        if (slot < 0) return;
        if (slot >= glyphs.cols * glyphs.rows) slot = 0;
        const sc = this.sc;
        sc.globalCompositeOperation = 'copy';
        sc.drawImage(glyphs.canvas, (slot % glyphs.cols) * cw, Math.floor(slot / glyphs.cols) * ch, cw, ch, 0, 0, cw, ch);
        sc.globalCompositeOperation = 'source-in';
        sc.fillStyle = 'rgb(' + fg[o] + ',' + fg[o + 1] + ',' + fg[o + 2] + ')';
        sc.fillRect(0, 0, cw, ch);
        pic.drawImage(this.scratch, x, y);
    };

    Painter.prototype.frame = function() { return frameOf(this); };
    Painter.prototype.origin = function() { return originOf(this); };

    Painter.prototype.draw = function() {
        const ctx = this.ctx;
        ctx.fillStyle = '#000';
        ctx.fillRect(0, 0, canvas.width, canvas.height);
        if (!this.cols || !this.masked) return;
        const [ox, oy] = this.origin();
        ctx.drawImage(this.picture, ox, oy);
        const s = stripOf(this);
        if (s) {
            ctx.imageSmoothingEnabled = false;
            ctx.drawImage(this.picture, 0, s.row * this.ch, s.cols * this.cw, this.ch, s.x, s.y, s.w, s.h);
        }
    };

    /* A program's font is a face of its own ahead of the page's stack, which still
       draws what the face lacks, and every glyph known is drawn again in it. The
       program's zoom is answered once the face is in, so its first frame is too. */
    let fontReady = Promise.resolve();
    let face = null;
    function loadFont(data) {
        const next = new FontFace(FACE, data);
        return next.load().then(function() {
            if (face) document.fonts.delete(face);
            face = next;
            document.fonts.add(face);
            font = '"' + FACE + '", ' + stack;
            drawer.rasterize(true);
            drawer.draw();
        }, function(err) {
            on.notice('Font not loaded (' + err.message + '): drawing in the page\'s own.');
        });
    }

    // === Fitting ===
    // cell sizes the drawer's cell to the device pixels cellBase covers, whole ones
    // and one wide to two tall, rasterizing the glyphs again when that changed
    function cell() {
        const dpr = window.devicePixelRatio || 1;
        const cw = Math.max(1, Math.round(cellBase * dpr / 2)), ch = 2 * cw;
        if (cw !== drawer.cw || ch !== drawer.ch) {
            drawer.cw = cw;
            drawer.ch = ch;
            drawer.rasterize(true);
        }
    }

    // size backs the canvas with the container's device pixels
    function size() {
        const dpr = window.devicePixelRatio || 1;
        const W = Math.max(1, Math.round(container.clientWidth * dpr)), H = Math.max(1, Math.round(container.clientHeight * dpr));
        if (canvas.width !== W || canvas.height !== H) {
            canvas.width = W;
            canvas.height = H;
        }
    }

    // fit sizes the grid to the cells the container holds at cellBase and reports it
    // to Go when it changed; a keyboard covering the page leaves the grid as it is
    function fit() {
        size();
        cell();
        const c = Math.max(1, Math.floor(canvas.width / drawer.cw));
        const r = Math.max(1, Math.floor(canvas.height / drawer.ch));
        if (c !== cols || r !== rows) {
            cols = c;
            rows = r;
            send({ type: 'resize', cols: cols, rows: rows });
        }
        drawer.draw();
        fitted();
        if (touch) layoutKeyboard();
    }

    // cellAt is the cell drawn under a pointer event, the nearest edge cell past the
    // frame
    function cellAt(e) {
        const rect = canvas.getBoundingClientRect();
        const dpr = window.devicePixelRatio || 1;
        const [ox, oy] = drawer.origin();
        const fc = drawer.cols || cols, fr = drawer.rows || rows;
        const x = Math.floor(((e.clientX - rect.left) * dpr - ox) / drawer.cw);
        const y = Math.floor(((e.clientY - rect.top) * dpr - oy) / drawer.ch);
        return [Math.min(Math.max(x, 0), fc - 1), Math.min(Math.max(y, 0), fr - 1)];
    }

    function initScreen() {
        if (typeof WebAssembly === 'undefined') {
            tell('failed', 'WebAssembly is not supported by this browser', 'wasm');
            return false;
        }
        try {
            drawer = new Drawer(canvas);
        } catch (err) {
            // Brave and other Chromium builds offer no WebGL on a blocklisted GPU
            try {
                drawer = new Painter(canvas);
            } catch (err2) {
                tell('failed', err2.message, 'canvas');
                return false;
            }
            tell('notice', 'No WebGL (' + err.message + '): drawing on a 2D canvas, slower on a large grid.');
        }
        if (touch) {
            if (strip) strip.hidden = false;   // before the first fit
            // Focus raises no keyboard while so marked; hardware keys still type through.
            input.setAttribute('inputmode', 'none');
        }
        fit();
        if (touch) wireTouch();
        return true;
    }

    // === Loading ===
    async function loadWasm(options) {
        let bytes, kept;
        try {
            const response = await fetch(options.wasm);
            // A 404's page would fail to compile as a bad magic word, which names nothing.
            if (!response.ok) throw new Error('HTTP ' + response.status);

            /* Buffered rather than instantiateStreaming, to report progress: a
               tee()'d body cannot be handed to instantiateStreaming because both
               branches share one source. Cost is that compilation starts after
               the download instead of during it. */
            if (response.body) {
                const reader = response.body.getReader();
                const chunks = [];
                let received = 0;
                let total = Number(response.headers.get('content-length')) || 0;
                for (;;) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    chunks.push(value);
                    received += value.length;
                    if (total && received > total) total = 0;   // compressed length; drop it
                    on.progress(received, total);
                }
                bytes = new Uint8Array(received);
                let offset = 0;
                for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length; }
            } else {
                bytes = new Uint8Array(await response.arrayBuffer());
            }
            kept = (await options.kept) || {};
        } catch (err) {
            console.error('TerminalPage: load failed', err);
            on.failed(err.message, 'load');
            return;
        }
        if (program === null) return;   // stopped while it loaded
        on.compiling();

        program = new Worker(options.worker || 'worker.js');
        program.onerror = function(e) {
            if (!program) return;   // a timer Go left behind throws after exit; the exit stands
            on.failed(e.message, 'start');
        };
        program.onmessage = onProgram;
        // Resolved here: the worker would resolve a relative name against its own URL.
        program.postMessage({ type: 'start', exec: new URL(options.exec || 'wasm_exec.js', document.baseURI).href,
            bytes: bytes.buffer, argv: options.argv || ['js'], cols: cols, rows: rows, kept: kept,
            features: options.features || [], hold: !!options.hold }, [bytes.buffer]);
        document.addEventListener('visibilitychange', function() {
            visibility();
            if (document.hidden) {
                lost();
                send({ type: 'blur' });
            }
        });
        wireHandlers();
    }

    // onProgram takes the worker's messages: its own, terminal's, and any other
    // kind, which is the program's to its page's script
    let framed = false;   // the first frame came
    function onProgram(e) {
        const msg = e.data;
        switch (msg.type) {
        case 'ready':
            on.ready();
            break;
        case 'frame': {
            const first = !framed;
            framed = true;
            // Every frame is reported drawn, or Go holds the next until it is.
            try {
                on.frame(first);
                drawer.apply(msg.data);
                drawer.draw();
            } finally {
                send({ type: 'drawn' });
            }
            break;
        }
        case 'pointer':
            pointer = { enabled: msg.enabled, motion: msg.motion };
            lastCell = null;
            break;
        case 'font':
            fontReady = loadFont(msg.data);
            break;
        case 'zoom':
            fontReady.then(function() { zoomFrom(msg); });
            break;
        case 'keep':
            on.keep(msg.name, msg.text);
            break;
        case 'exit':
            stop();
            on.exit(msg.code, msg.reason);
            break;
        case 'failed':
            stop();
            on.failed(msg.reason, 'load');
            break;
        default:
            on.message(msg);
        }
    }

    function run() {
        if (program) program.postMessage({ type: 'run' });
    }

    /* An ended program takes no more messages: a resize or a key posted to it
       would fail there, and its error would replace the reason it ended. */
    function stop() {
        const ended = program;
        program = null;
        if (!ended) return;
        ended.terminate();
        if (touch) input.blur();   // nothing left to type into, and a keyboard would cover what the page says
    }

    // The program draws nothing for a page nobody sees: a hidden document's, or one
    // its own script covers (setHidden)
    let veiled = false;
    function visibility() {
        if (program) program.postMessage({ type: 'visibility', hidden: document.hidden || veiled });
    }

    function focus() {
        input.focus({ preventScroll: true });
    }

    // === Keys ===
    /* Keys arrive on the hidden field: a hardware key as keydown and keyup by its
       code, a soft keyboard's or a paste's text as input events once the browser has
       composed it. A key the program takes never reaches the field; the rest stay the
       browser's: the chords a terminal emulator keeps for itself (Meta or Alt,
       Ctrl-Shift, F5, F11 and F12), paste, and a Ctrl chord a terminal has no byte for. */
    const NAMED = /^(Escape|Enter|Tab|Backspace|Delete|Arrow(Up|Down|Left|Right)|Home|End|Page(Up|Down)|Insert|F([1-9]|1[0-2]))$/;
    const CTRL = /^[a-zA-Z @[\\\]^_?]$/;   // the characters whose Ctrl chord a terminal sends
    const BROWSER = /^F(5|11|12)$/;        // the function keys the browser keeps

    function mods(e) {
        return (e.shiftKey ? 1 : 0) | (e.altKey ? 2 : 0) | (e.ctrlKey ? 4 : 0) | (e.metaKey ? 8 : 0);
    }

    function onKey(e) {
        on.gesture();
        if (e.isComposing || e.metaKey || e.altKey || (e.ctrlKey && e.shiftKey) || BROWSER.test(e.key)) return;
        if (e.shiftKey && e.key === 'Insert') return;   // pastes, as Ctrl-Shift-V does; the text arrives below
        const char = [...e.key].length === 1;
        if (!(char ? !e.ctrlKey || CTRL.test(e.key) : NAMED.test(e.key))) return;
        send({ type: 'key', key: e.key, mods: mods(e), code: e.code, repeat: e.repeat });
        e.preventDefault();
    }

    function onKeyUp(e) {
        if (e.code) send({ type: 'keyup', code: e.code });
    }

    function onInput(e) {
        if (e.isComposing || e.inputType === 'insertCompositionText') return;   // taken whole at its end
        switch (e.inputType) {
        case 'insertText':
            if (e.data) send({ type: 'text', text: e.data });
            break;
        case 'insertLineBreak':
        case 'insertParagraph':
            send({ type: 'key', key: 'Enter', mods: 0 });
            break;
        case 'deleteContentBackward':
            send({ type: 'key', key: 'Backspace', mods: 0 });
            break;
        }
        input.value = '';
    }

    function onCompositionEnd(e) {
        if (e.data) send({ type: 'text', text: e.data });
        input.value = '';
    }

    function onPaste(e) {
        const text = e.clipboardData && e.clipboardData.getData('text');
        if (text) send({ type: 'text', text: text });
        e.preventDefault();
    }

    // === Pointer ===
    /* The pointer is reported at its cell in the terminal's numbering: buttons left
       1, middle 2, right 3, wheel up 4 and down 5, back 6, forward 7; actions press 1,
       release 2, move 3, drag 4. A touch is the left button, marked as a finger's,
       and goes with the pointer off too, as Go keeps the bar's rows a finger presses
       then; motion once per cell, and only while Go asked for it. */
    let pointer = { enabled: false, motion: false };
    let lastCell = null;
    let fingerDown = false;   // a finger's press was sent and not let go
    const pressed = new Set();         // buttons a press was sent for and no release
    const BUTTONS = [1, 2, 3, 6, 7];   // PointerEvent.button to the terminal's
    const BITS = [1, 4, 2, 8, 16];     // PointerEvent.button to its bit in buttons

    function held(e) {
        if (e.pointerType === 'touch') return 1;
        return e.buttons & 1 ? 1 : e.buttons & 4 ? 2 : e.buttons & 2 ? 3 : 0;
    }

    function mouse(e, button, action) {
        const [x, y] = cellAt(e);
        send({ type: 'mouse', x: x, y: y, button: button, action: action, mods: mods(e),
               touch: e.pointerType === 'touch' });
        lastCell = [x, y];
        if (button === 4 || button === 5) return;   // a wheel's notch has no release
        if (action === 1) pressed.add(button);
        else if (action === 2) pressed.delete(button);
    }

    /* A pointer the page loses, to another tab, window or app, is let go of: its end
       reaches that one instead, so Go would act on for a press nobody holds, and a
       finger left listed would make every later tap the second of a gesture. Focus
       moving within the page, as a second button's press moves it, loses nothing. */
    function lost() {
        if (lastCell) {
            for (const button of pressed) {
                send({ type: 'mouse', x: lastCell[0], y: lastCell[1], button: button, action: 2, mods: 0, touch: fingerDown });
            }
        }
        pressed.clear();
        fingerDown = false;
        pointers.clear();
        reading.clear();
        pinch = null;
        bar.down = false;
        bar.taps = 0;
    }

    function onPointerDown(e) {
        on.gesture();
        focus();   // keys follow the pointer
        e.preventDefault();
        if (e.pointerType === 'touch' ? touchDown(e) : !pointer.enabled) return;
        const button = e.pointerType === 'touch' ? 1 : BUTTONS[e.button];
        if (!button) return;
        try {
            canvas.setPointerCapture(e.pointerId);
        } catch (err) {
            // a pointer the browser no longer counts active: the press still plays
        }
        mouse(e, button, 1);
        fingerDown = e.pointerType === 'touch';
    }

    function onPointerUp(e) {
        on.gesture();   // a touch's gesture is its release
        if (e.pointerType === 'touch') {
            if (!touchUp(e) && fingerDown) {
                fingerDown = false;
                mouse(e, 1, 2);
            }
            return;
        }
        if (!pointer.enabled) return;
        const button = BUTTONS[e.button];
        if (button) mouse(e, button, 2);
    }

    function onPointerMove(e) {
        if (e.pointerType === 'touch' && touchMove(e)) return;
        if (!pointer.enabled) return;
        // A button pressed or let go while another is held comes as a move
        if (e.pointerType !== 'touch' && e.button >= 0 && BUTTONS[e.button]) {
            mouse(e, BUTTONS[e.button], e.buttons & BITS[e.button] ? 1 : 2);
            return;
        }
        const button = held(e);
        if (!button && !pointer.motion) return;
        const [x, y] = cellAt(e);
        if (lastCell && lastCell[0] === x && lastCell[1] === y) return;
        mouse(e, button, button ? 4 : 3);
    }

    function onWheel(e) {
        e.preventDefault();
        if (e.ctrlKey) {   // a trackpad's pinch, or Ctrl and the wheel: zoom, never the page's
            if (e.deltaY) zoomTo(cellBase * (e.deltaY < 0 ? 1.1 : 1 / 1.1));
            return;
        }
        if (pointer.enabled && e.deltaY) mouse(e, e.deltaY < 0 ? 4 : 5, 1);
    }

    // === Zoom ===
    /* A zoom moves the cell the grid is fitted at, in the range's steps between its
       bounds, and refits: the grid changes, and Go sees a resize as it does for a
       window. Go is told the cell once it holds still, and may keep it. */
    const ZOOM_SETTLE_MS = 500;
    let zoom = null;   // { least, most, step }: nothing zooms before the program's zoom arrives
    let zoomFrame = 0, zoomSettle = 0;

    function zoomTo(cell) {
        if (!zoom) return;
        cell = Math.min(zoom.most, Math.max(zoom.least, Math.round(cell / zoom.step) * zoom.step));
        if (cell === cellBase) return;
        cellBase = cell;
        if (!zoomFrame) zoomFrame = requestAnimationFrame(function() { zoomFrame = 0; fit(); });
        clearTimeout(zoomSettle);
        zoomSettle = setTimeout(function() { send({ type: 'zoom', cell: cellBase }); }, ZOOM_SETTLE_MS);
    }

    // zoomFrom takes Go's range, the cell it opens at and the bar's rows, each 0 for
    // the page's own, and answers once the grid fits: Go waits for that before it
    // reads its size
    function zoomFrom(msg) {
        zoom = { least: msg.least || ZOOM.least, most: msg.most || ZOOM.most, step: msg.step || ZOOM.step };
        barRows = msg.bar || 0;
        if (msg.cell && msg.cell !== cellBase) {
            cellBase = msg.cell;
            fit();
        }
        send({ type: 'fitted' });
    }

    // === Fingers ===
    /* Two fingers on the canvas are a gesture, never play: the first's press is let go
       when the second lands, and moved apart or together past PINCH_SLOP they pinch the
       zoom. Three taps of one finger on the bar raise or drop the keyboard. A finger
       landing on the strip above a keyboard reads it and plays nothing, and stands for
       the bar while the keyboard is up. */
    const TAP_MS = 300, NEXT_TAP_MS = 500, PINCH_SLOP = 8;
    const BAR_TAPS = 3, BAR_SLOP = 100;   // how far a bar tap's finger may stray
    const BAR_REACH = 48;         // how near the frame's foot a bar tap may land, however thin the bar
    const pointers = new Map();   // fingers on the canvas, by id: where each is, and was as the pinch began
    const reading = new Set();    // fingers on the strip
    let pinch = null;             // { dist, cell, moved }: the pinch's start, and a finger past PINCH_SLOP
    const bar = { taps: 0, at: 0, down: false, x: 0, y: 0, since: 0 };   // one finger's taps on the bar
    let barRows = 0;              // the bar's, as Go names them
    let toggleKeyboard = function() {};

    function pinchDistance() {
        const [a, b] = [...pointers.values()];
        return Math.hypot(a.x - b.x, a.y - b.y);
    }

    // onStrip is a finger on the strip above a keyboard
    function onStrip(e) {
        const s = stripOf(drawer);
        return !!s && (e.clientY - canvas.getBoundingClientRect().top) * (window.devicePixelRatio || 1) >= s.y;
    }

    // onBar is a finger on the bar's rows, past the frame's foot, or within
    // BAR_REACH of it where the bar is drawn thinner
    function onBar(e) {
        if (barRows <= 0) return false;
        const dpr = window.devicePixelRatio || 1, [, oy] = drawer.origin();
        const foot = oy + (drawer.rows || rows) * drawer.ch;
        const y = (e.clientY - canvas.getBoundingClientRect().top) * dpr;
        return y >= foot - Math.max(barRows * drawer.ch, BAR_REACH * dpr);
    }

    // touchDown takes a finger the program does not get: one on the strip, or a second
    function touchDown(e) {
        const alone = pointers.size === 0 && reading.size === 0;
        bar.down = alone && (covered ? onStrip(e) : onBar(e));
        if (!alone) bar.taps = 0;
        bar.x = e.clientX;
        bar.y = e.clientY;
        bar.since = e.timeStamp;
        if (onStrip(e)) {
            reading.add(e.pointerId);
            return true;
        }
        pointers.set(e.pointerId, { x: e.clientX, y: e.clientY, x0: e.clientX, y0: e.clientY });
        if (pointers.size === 1) return false;
        if (fingerDown) {
            fingerDown = false;
            mouse(e, 1, 2);
        }
        for (const p of pointers.values()) { p.x0 = p.x; p.y0 = p.y; }
        if (pointers.size === 2) pinch = { dist: pinchDistance(), cell: cellBase, moved: false };
        return true;
    }

    // touchMove: a finger of two moved past the slop pinches from then on
    function touchMove(e) {
        if (bar.down && Math.hypot(e.clientX - bar.x, e.clientY - bar.y) > BAR_SLOP) bar.down = false;
        if (reading.has(e.pointerId)) return true;
        const p = pointers.get(e.pointerId);
        if (!p) return false;
        p.x = e.clientX;
        p.y = e.clientY;
        if (!pinch) return false;
        if (Math.abs(p.x - p.x0) > PINCH_SLOP || Math.abs(p.y - p.y0) > PINCH_SLOP) pinch.moved = true;
        if (pinch.moved && pointers.size === 2) zoomTo(pinch.cell * pinchDistance() / pinch.dist);
        return true;
    }

    // barUp ends a finger's tap on the bar, anywhere on it: the third in time
    // toggles the keyboard
    function barUp(e) {
        if (bar.down && e.type !== 'pointercancel' && e.timeStamp - bar.since <= TAP_MS) {
            bar.taps = e.timeStamp - bar.at <= NEXT_TAP_MS ? bar.taps + 1 : 1;
            bar.at = e.timeStamp;
        } else {
            bar.taps = 0;
        }
        if (bar.taps === BAR_TAPS) {
            bar.taps = 0;
            toggleKeyboard();
        }
        bar.down = false;
    }

    // touchUp ends a finger: the last of a pinch's ends it
    function touchUp(e) {
        barUp(e);
        if (reading.delete(e.pointerId)) return true;
        pointers.delete(e.pointerId);
        if (!pinch) return false;
        if (pointers.size === 0) pinch = null;
        return true;
    }

    // === Event Wiring ===
    function wireHandlers() {
        input.addEventListener('keydown', onKey);
        input.addEventListener('keyup', onKeyUp);
        input.addEventListener('blur', function() { send({ type: 'blur' }); });
        window.addEventListener('blur', lost);
        input.addEventListener('input', onInput);
        input.addEventListener('compositionend', onCompositionEnd);
        input.addEventListener('paste', onPaste);

        canvas.addEventListener('pointerdown', onPointerDown);
        canvas.addEventListener('pointerup', onPointerUp);
        canvas.addEventListener('pointercancel', onPointerUp);
        canvas.addEventListener('pointermove', onPointerMove);
        canvas.addEventListener('wheel', onWheel, { passive: false });
        canvas.addEventListener('contextmenu', function(e) { e.preventDefault(); });

        let fitTimer = 0;
        function handleResize() {
            // Where a soft keyboard shrinks the page itself, the grid must not follow it.
            if ((keyboard || keyboardUp) && !pageChanged()) {
                refitPending = true;
                return;
            }
            clearTimeout(fitTimer);
            fitTimer = setTimeout(fit, 50);
        }

        // A resolution query matches one ratio only, so re-arm on every change.
        let dprQuery = null;
        function onDprChange() { watchDpr(); handleResize(); }
        function watchDpr() {
            if (dprQuery) dprQuery.removeEventListener('change', onDprChange);
            dprQuery = window.matchMedia('(resolution: ' + window.devicePixelRatio + 'dppx)');
            dprQuery.addEventListener('change', onDprChange);
        }

        new ResizeObserver(handleResize).observe(container);
        watchDpr();

        // Focus for keyboard capture; an iframe may need a click first
        focus();
    }

    // === Touch: key strip and system keyboard ===
    const KEYBOARD_MIN = 100;   // px of screen a soft keyboard takes at least; less is a toolbar
    let keyboard = false;       // the player asked for the system keyboard
    let keyboardSeen = false;   // and it came up, so its going away is the system dismissing it
    let keyboardUp = false;     // a keyboard the player asked for covers the page, or is still leaving
    let refitPending = false;   // a resize put off while that keyboard was asked for or up
    let fitW = 0, fitH = 0;     // page size the grid was last fitted to
    let fitFull = false;        // and whether it was fullscreen then
    let layoutKeyboard = function() {};
    let setKeyboard = function() {};

    function fitted() {
        fitW = document.documentElement.clientWidth;
        fitH = document.documentElement.clientHeight;
        fitFull = document.fullscreenElement != null;
        keyboardSeen = false;   // seen over the old page; the layout after the fit looks again
        refitPending = false;   // this fit took the resizes put off before it
    }

    // A rotation or a fullscreen change: the page itself resized, which no keyboard does.
    function pageChanged() {
        return document.documentElement.clientWidth !== fitW
            || (document.fullscreenElement != null) !== fitFull;
    }

    function wireTouch() {
        const vv = window.visualViewport;

        if (strip) {
            // A strip tap must leave focus, and with it the keyboard, where it is.
            strip.addEventListener('pointerdown', function(e) { e.preventDefault(); });
            strip.addEventListener('click', function(e) {
                const b = e.target.closest('button');
                if (b && b.dataset.key) send({ type: 'key', key: b.dataset.key, mods: Number(b.dataset.mods) || 0 });
            });
        }

        setKeyboard = function(want, refocus) {
            keyboard = want;
            keyboardSeen = false;
            input.setAttribute('inputmode', want ? 'text' : 'none');
            // A focused field keeps the keyboard it has: refocusing inside the tap is what
            // raises or drops it, and the only way iOS raises one at all.
            if (refocus) {
                input.blur();
                focus();
            }
            on.keyboard(want);
            layoutKeyboard();
        };

        toggleKeyboard = function() { setKeyboard(!keyboard, true); };

        /* While a keyboard covers the page the grid keeps its size, and the strip
           shows the last row above the keyboard; the program is told, and keeps its
           input to the strip's half. */
        layoutKeyboard = function() {
            if (pageChanged()) return;   // measured against the old page; the refit lays out again
            const up = !!vv && fitH - vv.height * vv.scale > KEYBOARD_MIN;
            if (keyboard && up) keyboardSeen = true;
            else if (keyboard && keyboardSeen) return setKeyboard(false, false);   // dismissed by the system
            keyboardUp = up && (keyboard || keyboardUp);
            // Only once the page is whole again does a put-off refit see its real size.
            if (!up && !keyboard && refitPending) {
                refitPending = false;
                fit();
            }
            // The page follows the visible area if the browser pans it to the keyboard.
            document.body.style.transform = up && vv.offsetTop ? 'translateY(' + vv.offsetTop + 'px)' : '';
            const above = strip ? strip.offsetHeight : 0;
            const visible = Math.round((vv ? vv.height - above : 0) * (window.devicePixelRatio || 1));
            setCovered(up ? canvas.height - visible : 0);
        };

        if (vv) {
            let queued = false;
            const schedule = function() {
                if (queued) return;
                queued = true;
                requestAnimationFrame(function() { queued = false; layoutKeyboard(); });
            };
            vv.addEventListener('resize', schedule);
            vv.addEventListener('scroll', schedule);
        }

        // Fullscreen refits the grid as a rotation does, even where the size holds.
        document.addEventListener('fullscreenchange', fit);
    }

    // === Entry point ===
    const page = {
        touch: touch,
        get running() { return !!program; },
        run: run,
        send: send,
        stop: stop,
        focus: focus,
        lost: lost,
        fit: function() { if (drawer) fit(); },
        setHidden: function(hidden) { veiled = !!hidden; visibility(); },
        setKeyboard: function(want) { setKeyboard(!!want, true); },
        toggleKeyboard: function() { toggleKeyboard(); },
    };

    // open draws the screen at once and starts the program's download; a document
    // holds one page
    function open(options) {
        if (canvas) throw new Error('TerminalPage: this document has a page open');
        container = options.container;
        canvas = options.canvas;
        input = options.input;
        strip = options.strip || null;
        for (const name of CALLBACKS) on[name] = (options.on && options.on[name]) || function() {};
        if (options.cell) cellBase = options.cell;
        if (options.font) font = stack = options.font;
        if (initScreen()) loadWasm(options);
        return page;
    }

    window.TerminalPage = { open: open, touch: touch, mods: mods };
})();
