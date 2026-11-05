import { useEffect, useRef, useState } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

const BANNER = [
  '\x1b[1;36mENTROPY\x1b[0m  isolated sandbox  \x1b[2m(no network, 256 MB, 0.5 CPU, 100 processes)\x1b[0m',
  '\x1b[2mcd and export persist between commands. Ctrl+C interrupts, Ctrl+L clears.\x1b[0m',
  '\x1b[2mMulti-line pastes run as one script. New here? Start with "First steps" in the labs panel.\x1b[0m',
  '',
];

function promptLength(cwd) {
  return `root@entropy:${cwd || '~'}# `.length;
}

function prompt(cwd) {
  return `\x1b[1;32mroot@entropy\x1b[0m:\x1b[1;34m${cwd || '~'}\x1b[0m# `;
}

// Terminal is a line-editing shell front end. Commands run remotely, one per
// exec, so the line editor lives here: cursor movement, history, kill-line,
// paste, and Ctrl+C all behave like a normal shell prompt.
export default function Terminal({
  subscribe,
  onCommand,
  onInterrupt,
  enabled,
  busy,
  cwd,
  restoredHistory,
  ptyUrl,
}) {
  const hostRef = useRef(null);
  const termRef = useRef(null);
  const fitRef = useRef(null);
  const state = useRef({
    buffer: '',
    cursor: 0,
    history: [],
    historyIndex: -1,
    draft: '',
    atPrompt: false,
    running: false,
    lineOpen: false,
    pendingSubmit: false,
    cursorRow: 0,
    cwd: cwd || '~',
  });
  const props = useRef({ onCommand, onInterrupt, enabled });
  props.current = { onCommand, onInterrupt, enabled };
  // mode is 'line' (local line editor, one exec per command) or 'pty' (raw
  // bytes to an interactive shell). 'exited' waits for Enter to restart.
  const modeRef = useRef('line');
  const ptyRef = useRef(null);
  const lineApi = useRef(null);
  const [mode, setMode] = useState('line');

  useEffect(() => {
    state.current.cwd = cwd || '~';
  }, [cwd]);

  useEffect(() => {
    if (restoredHistory?.length && state.current.history.length === 0) {
      state.current.history = [...restoredHistory].reverse();
    }
  }, [restoredHistory]);

  useEffect(() => {
    const term = new XTerm({
      theme: {
        background: '#0e1116',
        foreground: '#d7dce2',
        cursor: '#3bb273',
        cursorAccent: '#0e1116',
        selectionBackground: '#1f6b5c66',
        black: '#0e1116',
        red: '#f97066',
        green: '#3bb273',
        yellow: '#d4a017',
        blue: '#6b9bd1',
        magenta: '#b392c9',
        cyan: '#5fb3b3',
        white: '#d7dce2',
        brightBlack: '#5c6570',
      },
      fontFamily: 'IBM Plex Mono, ui-monospace, monospace',
      fontSize: 13,
      lineHeight: 1.25,
      cursorBlink: true,
      convertEol: true,
      scrollback: 5000,
      allowProposedApi: false,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(hostRef.current);
    termRef.current = term;
    fitRef.current = fit;

    const s = state.current;
    BANNER.forEach((line) => term.writeln(line));

    // redraw repaints the prompt and input line. It is wrap-aware: it moves
    // up to the first row of the previous render, clears to the end of the
    // screen, repaints, and places the cursor by row and column.
    const redraw = () => {
      const cols = Math.max(1, term.cols);
      const promptLen = promptLength(s.cwd);
      let out = '';
      if (s.cursorRow > 0) out += `\x1b[${s.cursorRow}A`;
      out += `\r\x1b[J${prompt(s.cwd)}${s.buffer}`;
      const total = promptLen + s.buffer.length;
      const endRow = Math.floor(total / cols);
      // At an exact multiple of the width xterm defers the wrap. Writing a
      // space forces it, so the cursor ends on endRow either way; then return
      // to column 0 of that row.
      out += total > 0 && total % cols === 0 ? ' \r' : '\r';
      const target = promptLen + s.cursor;
      const targetRow = Math.floor(target / cols);
      const targetCol = target % cols;
      if (endRow > targetRow) out += `\x1b[${endRow - targetRow}A`;
      if (targetCol > 0) out += `\x1b[${targetCol}C`;
      s.cursorRow = targetRow;
      term.write(out);
    };
    const showPrompt = () => {
      s.atPrompt = true;
      redraw();
    };
    const leavePrompt = () => {
      if (s.atPrompt) {
        term.write(`${s.cursorRow > 0 ? `\x1b[${s.cursorRow}A` : ''}\r\x1b[J`);
        s.cursorRow = 0;
        s.atPrompt = false;
      }
    };

    const submit = (text) => {
      const command = text.replace(/\s+$/, '');
      s.buffer = '';
      s.cursor = 0;
      s.historyIndex = -1;
      s.draft = '';
      if (!command.trim()) {
        term.write('\r\n');
        s.cursorRow = 0;
        showPrompt();
        return;
      }
      if (s.history[0] !== command) s.history.unshift(command);
      s.history = s.history.slice(0, 200);
      term.write('\r\n');
      s.cursorRow = 0;
      s.atPrompt = false;
      if (!props.current.enabled) {
        term.writeln('\x1b[33mnot connected; command not sent\x1b[0m');
        showPrompt();
        return;
      }
      s.running = true;
      if (props.current.onCommand(command) === false) {
        s.running = false;
        term.writeln('\x1b[33mnot connected; command not sent\x1b[0m');
        showPrompt();
      }
    };

    const insert = (text) => {
      s.buffer = s.buffer.slice(0, s.cursor) + text + s.buffer.slice(s.cursor);
      s.cursor += text.length;
      redraw();
    };

    const setFromHistory = (index) => {
      if (index === -1) {
        s.buffer = s.draft;
      } else {
        if (s.historyIndex === -1) s.draft = s.buffer;
        s.buffer = s.history[index];
      }
      s.historyIndex = index;
      s.cursor = s.buffer.length;
      redraw();
    };

    const deleteWordBack = () => {
      const left = s.buffer.slice(0, s.cursor).replace(/\S+\s*$/, '');
      s.buffer = left + s.buffer.slice(s.cursor);
      s.cursor = left.length;
      redraw();
    };

    const onData = (data) => {
      if (modeRef.current === 'pty') {
        const sock = ptyRef.current;
        if (sock?.readyState === WebSocket.OPEN) sock.send(new TextEncoder().encode(data));
        return;
      }
      if (modeRef.current === 'exited') {
        if (data === '\r') lineApi.current?.restartPTY();
        return;
      }
      if (s.running) {
        // Typeahead: keep editing the next command silently, and run it when
        // the current one finishes, the way a local shell does.
        if (data === '\x03') {
          term.write('^C');
          s.buffer = '';
          s.cursor = 0;
          s.pendingSubmit = false;
          props.current.onInterrupt();
        } else if (data === '\r') {
          s.pendingSubmit = true;
        } else if (data === '\x7f' || data === '\b') {
          s.buffer = s.buffer.slice(0, -1);
          s.cursor = s.buffer.length;
        } else if (!data.startsWith('\x1b')) {
          s.buffer += data.replace(/[\x00-\x1f\x7f]/g, '');
          s.cursor = s.buffer.length;
        }
        return;
      }
      if (!s.atPrompt) showPrompt();

      // Multi-line paste: run the whole block as one script.
      if (data.length > 1 && /[\r\n]/.test(data.replace(/[\r\n]+$/, ''))) {
        const script = (s.buffer + data).replace(/\r\n?/g, '\n').replace(/\n+$/, '');
        const lines = script.split('\n');
        term.write(`${s.cursorRow > 0 ? `\x1b[${s.cursorRow}A` : ''}\r\x1b[J${prompt(s.cwd)}${lines[0]}`);
        s.cursorRow = 0;
        lines.slice(1).forEach((line) => term.write(`\r\n\x1b[2m>\x1b[0m ${line}`));
        submit(script);
        return;
      }

      switch (data) {
        case '\r':
          s.cursor = s.buffer.length;
          redraw();
          submit(s.buffer);
          return;
        case '\x7f':
        case '\b':
          if (s.cursor > 0) {
            s.buffer = s.buffer.slice(0, s.cursor - 1) + s.buffer.slice(s.cursor);
            s.cursor -= 1;
            redraw();
          }
          return;
        case '\x1b[3~':
          if (s.cursor < s.buffer.length) {
            s.buffer = s.buffer.slice(0, s.cursor) + s.buffer.slice(s.cursor + 1);
            redraw();
          }
          return;
        case '\x03':
          s.cursor = s.buffer.length;
          redraw();
          term.write('^C\r\n');
          s.cursorRow = 0;
          s.buffer = '';
          s.cursor = 0;
          s.historyIndex = -1;
          showPrompt();
          return;
        case '\x0c':
          term.clear();
          term.write('\x1b[H\x1b[2J');
          s.cursorRow = 0;
          redraw();
          return;
        case '\x15':
          s.buffer = s.buffer.slice(s.cursor);
          s.cursor = 0;
          redraw();
          return;
        case '\x0b':
          s.buffer = s.buffer.slice(0, s.cursor);
          redraw();
          return;
        case '\x17':
          deleteWordBack();
          return;
        case '\x01':
        case '\x1b[H':
        case '\x1bOH':
          s.cursor = 0;
          redraw();
          return;
        case '\x05':
        case '\x1b[F':
        case '\x1bOF':
          s.cursor = s.buffer.length;
          redraw();
          return;
        case '\x1b[A':
          if (s.historyIndex < s.history.length - 1) setFromHistory(s.historyIndex + 1);
          return;
        case '\x1b[B':
          if (s.historyIndex > -1) setFromHistory(s.historyIndex - 1);
          return;
        case '\x1b[D':
          if (s.cursor > 0) {
            s.cursor -= 1;
            redraw();
          }
          return;
        case '\x1b[C':
          if (s.cursor < s.buffer.length) {
            s.cursor += 1;
            redraw();
          }
          return;
        case '\t':
          insert('  ');
          return;
        default:
          break;
      }
      if (data.startsWith('\x1b')) return;
      const printable = data.replace(/[\x00-\x1f\x7f]/g, '');
      if (printable) insert(printable);
    };

    const disposable = term.onData(onData);

    const unsubscribe = subscribe((event) => {
      // In interactive mode the shell draws prompts and output itself; only
      // output from line-mode execs (rare fallbacks) is written here.
      if (modeRef.current !== 'line') {
        if (event.type === 'output' && !event.notice) term.write(event.text);
        return;
      }
      switch (event.type) {
        case 'output':
          leavePrompt();
          if (event.notice) {
            term.write(`\x1b[2m${event.text.replace(/^\r?\n/, '')}\x1b[0m`);
            if (!s.running) {
              if (!event.text.endsWith('\n')) term.write('\r\n');
              showPrompt();
            }
          } else {
            term.write(event.text);
            s.lineOpen = !/\n$/.test(event.text);
          }
          break;
        case 'complete':
          leavePrompt();
          s.running = false;
          if (event.cwd) s.cwd = event.cwd;
          // Start the prompt on a fresh line after output without a newline.
          if (s.lineOpen) term.write('\r\n');
          s.lineOpen = false;
          if (event.exitCode !== 0) term.writeln(`\x1b[2;31m[exit ${event.exitCode}]\x1b[0m`);
          showPrompt();
          if (s.pendingSubmit) {
            s.pendingSubmit = false;
            submit(s.buffer);
          }
          break;
        case 'crash':
          leavePrompt();
          if (s.lineOpen) term.write('\r\n');
          s.lineOpen = false;
          term.writeln(`\x1b[1;31mcrash\x1b[0m \x1b[31m${event.message}\x1b[0m`);
          term.writeln('\x1b[2mAn explanation is streaming into the panel below.\x1b[0m');
          if (!s.running) showPrompt();
          break;
        case 'error':
          leavePrompt();
          if (s.lineOpen) term.write('\r\n');
          s.lineOpen = false;
          term.writeln(`\x1b[33m${event.text}\x1b[0m`);
          s.running = false;
          showPrompt();
          break;
        default:
          break;
      }
    });

    showPrompt();
    lineApi.current = { showPrompt, term, fit };

    // Fit on the next frame and only when the host is visible; fitting a
    // detached or zero-size terminal throws inside xterm's renderer.
    let disposed = false;
    let frame = 0;
    const onResize = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const host = hostRef.current;
        if (disposed || !host || host.offsetWidth === 0 || host.offsetHeight === 0) return;
        try {
          fit.fit();
        } catch {
          /* renderer not ready */
        }
        const sock = ptyRef.current;
        if (modeRef.current === 'pty' && sock?.readyState === WebSocket.OPEN) {
          sock.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
        }
      });
    };
    const observer = new ResizeObserver(onResize);
    observer.observe(hostRef.current);

    return () => {
      disposed = true;
      cancelAnimationFrame(frame);
      observer.disconnect();
      unsubscribe();
      disposable.dispose();
      term.dispose();
    };
    // The terminal is created once; callbacks are read through refs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [subscribe]);

  // Interactive terminal: connect once the session is live. On failure the
  // line editor stays in charge, so the terminal always works.
  useEffect(() => {
    if (!ptyUrl || !termRef.current) return undefined;
    const term = termRef.current;
    let closedByUs = false;
    let attempts = 0;
    let timer = 0;

    const setModeBoth = (m) => {
      modeRef.current = m;
      setMode(m);
      // Full-screen programs move the cursor with bare line feeds; only the
      // line editor wants LF translated to CRLF.
      term.options.convertEol = m === 'line';
    };

    const connect = () => {
      const sep = ptyUrl.includes('?') ? '&' : '?';
      const sock = new WebSocket(`${ptyUrl}${sep}cols=${term.cols}&rows=${term.rows}`);
      sock.binaryType = 'arraybuffer';
      ptyRef.current = sock;
      let opened = false;
      sock.onopen = () => {
        opened = true;
        attempts = 0;
        if (modeRef.current === 'line') {
          // Hand the screen to the shell: clear the local prompt line.
          term.write('\r\x1b[2K');
        }
        setModeBoth('pty');
        term.focus();
      };
      sock.onmessage = (e) => {
        if (typeof e.data === 'string') {
          try {
            const msg = JSON.parse(e.data);
            if (msg.type === 'exit') {
              setModeBoth('exited');
              term.write(`\r\n\x1b[2mshell exited (${msg.code}). Press Enter for a new shell.\x1b[0m\r\n`);
            }
          } catch {
            /* ignore */
          }
          return;
        }
        term.write(new Uint8Array(e.data));
      };
      sock.onclose = () => {
        if (closedByUs || modeRef.current === 'exited') return;
        if (attempts >= 4) {
          if (modeRef.current !== 'line') {
            setModeBoth('line');
            term.write('\r\n\x1b[33minteractive terminal unavailable; using line mode\x1b[0m\r\n');
            lineApi.current?.showPrompt();
          }
          return;
        }
        attempts += 1;
        timer = setTimeout(connect, 1000 * attempts);
      };
    };

    if (lineApi.current) {
      lineApi.current.restartPTY = () => {
        term.write('\r\n');
        setModeBoth('line');
        connect();
      };
    }
    connect();
    return () => {
      closedByUs = true;
      clearTimeout(timer);
      ptyRef.current?.close();
      ptyRef.current = null;
    };
  }, [ptyUrl]);

  return (
    <div className="term-well h-full flex flex-col">
      <div className="flex items-center justify-between gap-3 px-3 py-2 border-b border-[#1c222b]">
        <div className="flex items-center gap-2 min-w-0">
          <span
            className={`h-2 w-2 shrink-0 rounded-[2px] ${
              !enabled ? 'bg-[#5c6570]' : busy ? 'bg-[#d4a017]' : 'bg-[#3bb273]'
            }`}
            aria-hidden="true"
          />
          <span className="text-xs font-mono text-[#8b949e] truncate">root@entropy:{cwd || '~'}</span>
        </div>
        <div className="flex items-center gap-3 text-xs font-mono">
          <span className="text-[#8b949e]" title={mode === 'pty' ? 'Interactive shell: vim, top, and job control work' : 'Line mode: one command at a time'}>
            {mode === 'pty' ? 'interactive' : 'line mode'}
          </span>
          <span className="text-[#8b949e]">{!enabled ? 'offline' : busy ? 'running' : 'ready'}</span>
          {busy && enabled && mode === 'line' && (
            <button
              type="button"
              onClick={() => {
                termRef.current?.write('^C');
                onInterrupt();
              }}
              className="text-[#f97066] hover:text-white"
            >
              Interrupt
            </button>
          )}
          <button
            type="button"
            onClick={() => {
              termRef.current?.clear();
              termRef.current?.focus();
            }}
            className="text-[#8b949e] hover:text-white"
          >
            Clear
          </button>
        </div>
      </div>
      <div
        ref={hostRef}
        className="flex-1 min-h-0 px-3 py-2"
        onClick={() => termRef.current?.focus()}
        role="presentation"
      />
    </div>
  );
}
