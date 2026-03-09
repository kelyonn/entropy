import { useEffect, useRef } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

export default function Terminal({ sendCommand, output, crashData, connected }) {
  const terminalRef = useRef(null);
  const xtermRef = useRef(null);
  const inputBuffer = useRef('');
  const commandHistory = useRef([]);
  const historyIndex = useRef(-1);

  useEffect(() => {
    const term = new XTerm({
      theme: {
        background: '#0d1117',
        foreground: '#c9d1d9',
        cursor: '#58a6ff',
        red: '#ff7b72',
        green: '#3fb950',
        yellow: '#d29922',
      },
      fontFamily: 'JetBrains Mono, Fira Code, monospace',
      fontSize: 14,
      cursorBlink: true,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = term;

    // Welcome banner
    term.writeln('\x1b[1;34m╔═══════════════════════════════════════╗\x1b[0m');
    term.writeln('\x1b[1;34m║         ENTROPY - Crash & Learn        ║\x1b[0m');
    term.writeln('\x1b[1;34m╚═══════════════════════════════════════╝\x1b[0m');
    term.writeln('\x1b[33mTry: rm -rf / | kill -9 1 | :(){ :|:& };:\x1b[0m');
    term.writeln('');
    writePrompt(term);

    term.onKey(({ key, domEvent }) => {
      const code = domEvent.keyCode;

      if (code === 13) { // Enter
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

      } else if (code === 8) { // Backspace
        if (inputBuffer.current.length > 0) {
          inputBuffer.current = inputBuffer.current.slice(0, -1);
          term.write('\b \b');
        }

      } else if (code === 38) { // Arrow Up - history
        if (historyIndex.current < commandHistory.current.length - 1) {
          historyIndex.current++;
          const cmd = commandHistory.current[historyIndex.current];
          clearLine(term, inputBuffer.current);
          inputBuffer.current = cmd;
          term.write(cmd);
        }

      } else if (code === 40) { // Arrow Down
        if (historyIndex.current > 0) {
          historyIndex.current--;
          const cmd = commandHistory.current[historyIndex.current];
          clearLine(term, inputBuffer.current);
          inputBuffer.current = cmd;
          term.write(cmd);
        }

      } else { // Regular character
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
  }, []);

  // Write backend output to terminal
  useEffect(() => {
    if (output && xtermRef.current) {
      xtermRef.current.write(output);
      writePrompt(xtermRef.current);
    }
  }, [output]);

  // Highlight crash in terminal
  useEffect(() => {
    if (crashData && xtermRef.current) {
      xtermRef.current.writeln('\x1b[1;31m💥 CRASH DETECTED\x1b[0m');
      xtermRef.current.writeln(`\x1b[31m${crashData}\x1b[0m`);
      xtermRef.current.writeln('\x1b[33m🤖 Entropy AI is analyzing...\x1b[0m');
    }
  }, [crashData]);

  return (
    <div className="h-full bg-gray-950 rounded-lg border border-gray-700 p-2">
      <div className="flex items-center gap-2 mb-2 px-2">
        <div className="w-3 h-3 rounded-full bg-red-500"/>
        <div className="w-3 h-3 rounded-full bg-yellow-500"/>
        <div className="w-3 h-3 rounded-full bg-green-500"/>
        <span className="text-gray-400 text-xs ml-2 font-mono">
          entropy@sandbox — {connected ? '🟢 connected' : '🔴 disconnected'}
        </span>
      </div>
      <div ref={terminalRef} className="h-full"/>
    </div>
  );
}

function writePrompt(term) {
  term.write('\x1b[1;32muser@entropy\x1b[0m:\x1b[1;34m/sandbox\x1b[0m$ ');
}

function clearLine(term, currentInput) {
  for (let i = 0; i < currentInput.length; i++) {
    term.write('\b \b');
  }
}
