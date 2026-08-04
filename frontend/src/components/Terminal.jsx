import { useEffect, useRef } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

export default function Terminal({ sendCommand, outputChunk, exitCode, crashMessage, connected, busy }) {
  const terminalRef = useRef(null);
  const xtermRef = useRef(null);
  const inputBuffer = useRef('');
  const commandHistory = useRef([]);
  const historyIndex = useRef(-1);
  const lastOutputSeq = useRef(0);
  const lastExitRef = useRef(null);
  const lastCrashRef = useRef(null);

  useEffect(() => {
    const term = new XTerm({
      theme: {
        background: '#0e1116',
        foreground: '#d7dce2',
        cursor: '#1f6b5c',
        selectionBackground: '#1f6b5c55',
        red: '#f97066',
        green: '#3bb273',
        yellow: '#d4a017',
        blue: '#6b9bd1',
      },
      fontFamily: 'IBM Plex Mono, ui-monospace, monospace',
      fontSize: 13,
      cursorBlink: true,
      convertEol: true,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalRef.current);
    fitAddon.fit();
    xtermRef.current = term;

    term.writeln('\x1b[1;36mENTROPY\x1b[0m  isolated sandbox terminal');
    term.writeln('\x1b[33mExamples: rm -rf /tmp/demo  |  kill -9 1  |  :(){ :|:& };:\x1b[0m');
    term.writeln('');
    writePrompt(term);

    term.onKey(({ key, domEvent }) => {
      const code = domEvent.keyCode;

      if (code === 13) {
        const cmd = inputBuffer.current.trim();
        if (cmd) {
          commandHistory.current.unshift(cmd);
          historyIndex.current = -1;
          term.writeln('');
          sendCommand(cmd);
        } else {
          term.writeln('');
          writePrompt(term);
        }
        inputBuffer.current = '';
      } else if (code === 8) {
        if (inputBuffer.current.length > 0) {
          inputBuffer.current = inputBuffer.current.slice(0, -1);
          term.write('\b \b');
        }
      } else if (code === 38) {
        if (historyIndex.current < commandHistory.current.length - 1) {
          historyIndex.current++;
          const cmd = commandHistory.current[historyIndex.current];
          clearLine(term, inputBuffer.current);
          inputBuffer.current = cmd;
          term.write(cmd);
        }
      } else if (code === 40) {
        if (historyIndex.current > 0) {
          historyIndex.current--;
          const cmd = commandHistory.current[historyIndex.current];
          clearLine(term, inputBuffer.current);
          inputBuffer.current = cmd;
          term.write(cmd);
        } else if (historyIndex.current === 0) {
          historyIndex.current = -1;
          clearLine(term, inputBuffer.current);
          inputBuffer.current = '';
        }
      } else if (key.length === 1 && !domEvent.ctrlKey && !domEvent.metaKey) {
        inputBuffer.current += key;
        term.write(key);
      }
    });

    const handleResize = () => fitAddon.fit();
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      term.dispose();
    };
  }, [sendCommand]);

  useEffect(() => {
    if (!outputChunk || !xtermRef.current) return;
    if (outputChunk.seq <= lastOutputSeq.current) return;
    lastOutputSeq.current = outputChunk.seq;
    xtermRef.current.write(outputChunk.text);
  }, [outputChunk]);

  useEffect(() => {
    if (exitCode == null || !xtermRef.current) return;
    if (exitCode === lastExitRef.current) return;
    lastExitRef.current = exitCode;
    if (exitCode !== 0) {
      xtermRef.current.writeln(`\x1b[31mexit ${exitCode}\x1b[0m`);
    }
    writePrompt(xtermRef.current);
  }, [exitCode]);

  useEffect(() => {
    if (!crashMessage || !xtermRef.current) return;
    if (crashMessage === lastCrashRef.current) return;
    lastCrashRef.current = crashMessage;
    xtermRef.current.writeln('\x1b[1;31mCRASH DETECTED\x1b[0m');
    xtermRef.current.writeln(`\x1b[31m${crashMessage}\x1b[0m`);
    xtermRef.current.writeln('\x1b[33mAnalyzing failure...\x1b[0m');
  }, [crashMessage]);

  return (
    <div className="term-well h-full flex flex-col p-3">
      <div className="flex items-center justify-between mb-2 px-1">
        <span className="text-xs font-mono text-[#8b949e]">root@entropy:~</span>
        <span className="text-xs font-mono text-[#8b949e]">
          {busy ? 'busy' : connected ? 'connected' : 'disconnected'}
        </span>
      </div>
      <div ref={terminalRef} className="flex-1 min-h-0" />
    </div>
  );
}

function writePrompt(term) {
  term.write('\x1b[1;32mroot@entropy\x1b[0m:\x1b[1;34m~\x1b[0m# ');
}

function clearLine(term, currentInput) {
  for (let i = 0; i < currentInput.length; i++) {
    term.write('\b \b');
  }
}
