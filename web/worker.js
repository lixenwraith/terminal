/* The Go program's thread. The page draws the frames posted from here and reports
   back what it saw: keys, the pointer, its size and each frame drawn. This side
   presents both to Go as the one global terminalPage, and keeps Go's stderr for the
   page to show once the program ends. */
(function() {
    'use strict';

    /* Why the program ended. stderr is a Go program's only word on that, and
       wasm_exec sends it to the console alone; kept here, the page shows it once the
       program exits instead of leaving a screen that simply stopped. */
    let lastLine = '';
    let crashLine = '';
    const errDecoder = new TextDecoder('utf-8');
    let partial = '';

    // wasm_exec.js by the URL the page names, then its stderr kept as above.
    function loadRuntime(exec) {
        importScripts(exec);
        const writeSync = globalThis.fs.writeSync;
        globalThis.fs.writeSync = function(fd, buf) {
            if (fd === 2) {
                const lines = (partial + errDecoder.decode(buf, { stream: true })).split('\n');
                partial = lines.pop();
                for (const line of lines.map(l => l.trim()).filter(Boolean)) {
                    // A crash's first line names it; the rest is its stack.
                    if (!crashLine && /^(panic|fatal error):/.test(line)) crashLine = line;
                    lastLine = line;
                }
            }
            return writeSync.call(this, fd, buf);
        };
    }

    /* The bridge: what Go reads of the page, what it calls to reach it, and event,
       which Go sets to take the page's messages. Go makes each array for one call
       alone, so its buffer moves to the page. A kind terminal does not know is the
       program's own to its page's script, and passes as it is. */
    globalThis.terminalPage = {
        grid: { cols: 80, rows: 24 },
        kept: {},
        features: [],
        hidden: false,
        frame: function(data) {
            postMessage({ type: 'frame', data: data }, [data.buffer]);
        },
        post: function(kind, fields) {
            const moved = [];
            for (const name in fields) {
                if (ArrayBuffer.isView(fields[name])) moved.push(fields[name].buffer);
            }
            postMessage(Object.assign({}, fields, { type: kind }), moved);
        },
        event: null,
    };

    // The program compiled, until it runs: at once, or at the page's run where it holds
    let held = null;

    function run() {
        if (!held) return;
        const go = held;
        held = null;
        go();
    }

    async function start(msg) {
        try {
            loadRuntime(msg.exec || 'wasm_exec.js');
        } catch (err) {
            postMessage({ type: 'failed', reason: 'wasm_exec.js did not load: ' + err.message });
            return;
        }
        const page = globalThis.terminalPage;
        page.grid = { cols: msg.cols, rows: msg.rows };
        page.kept = msg.kept || {};
        page.features = msg.features || [];
        const go = new Go();
        go.argv = msg.argv;
        go.exit = function(code) {
            postMessage({ type: 'exit', code: code, reason: crashLine || lastLine });
        };
        try {
            const result = await WebAssembly.instantiate(msg.bytes, go.importObject);
            held = function() { go.run(result.instance); };
            postMessage({ type: 'ready' });
        } catch (err) {
            postMessage({ type: 'failed', reason: err.message });
            return;
        }
        if (!msg.hold) run();
    }

    self.onmessage = function(e) {
        const msg = e.data, page = globalThis.terminalPage;
        switch (msg.type) {
        case 'start':
            start(msg);
            break;
        case 'run':
            run();
            break;
        case 'event':
            // A size measured before the program started is what it opens at.
            if (msg.event.type === 'resize') {
                page.grid.cols = msg.event.cols;
                page.grid.rows = msg.event.rows;
            }
            if (typeof page.event === 'function') page.event(msg.event);
            break;
        case 'visibility':
            /* A program that draws nothing while hidden (terminal.PageHidden) leaves no
               frames queued behind the page's throttled timers to replay on return; the
               next frame it draws once seen again is the whole screen. */
            page.hidden = msg.hidden;
            if (!msg.hidden && typeof page.event === 'function') page.event({ type: 'sync' });
            break;
        }
    };
})();
