import { useEffect, useState } from 'react';
import axios from 'axios';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

const API_URL = import.meta.env.VITE_API_URL || '';

// Postmortem shows the generated, blameless postmortem for a lab run (or the
// whole session) with copy, download, and replay-sharing actions.
export default function Postmortem({ sessionId, labId, authHeaders, onClose }) {
  const [markdown, setMarkdown] = useState('');
  const [error, setError] = useState('');
  const [status, setStatus] = useState('');

  useEffect(() => {
    axios
      .get(`${API_URL}/api/postmortem`, { headers: authHeaders(), params: { session_id: sessionId, lab: labId || undefined } })
      .then((res) => setMarkdown(res.data.markdown || ''))
      .catch(() => setError('The postmortem could not be generated.'));
  }, [sessionId, labId, authHeaders]);

  useEffect(() => {
    const onKey = (e) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const flash = (msg) => {
    setStatus(msg);
    setTimeout(() => setStatus(''), 2500);
  };

  const copy = async (text, msg) => {
    try {
      await navigator.clipboard.writeText(text);
      flash(msg);
    } catch {
      flash('Copy failed; select the text instead.');
    }
  };

  const download = () => {
    const blob = new Blob([markdown], { type: 'text/markdown' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `postmortem-${labId || 'session'}.md`;
    a.click();
    URL.revokeObjectURL(a.href);
  };

  const share = async () => {
    try {
      const res = await axios.post(`${API_URL}/api/replay/share`, null, {
        headers: authHeaders(),
        params: { session_id: sessionId },
      });
      const lab = labId ? `&lab=${encodeURIComponent(labId)}` : '';
      await copy(`${window.location.origin}/?replay=${res.data.token}${lab}`, 'Replay link copied. Anyone with it can watch, read-only.');
    } catch {
      flash('Could not create a replay link.');
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[#14181f]/30 p-4" onClick={onClose} role="presentation">
      <div
        className="panel max-w-3xl w-full max-h-[88vh] flex flex-col"
        role="dialog"
        aria-modal="true"
        aria-label="Postmortem"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between gap-3 px-5 py-3 border-b border-line">
          <div>
            <div className="text-sm font-semibold text-ink">Postmortem</div>
            <div className="text-xs text-ink-muted">Generated from your recorded timeline. Edit it before you share it.</div>
          </div>
          <button type="button" onClick={onClose} className="text-xs text-ink-muted hover:text-ink">
            Close
          </button>
        </header>
        <div className="px-6 py-4 overflow-y-auto prose-entropy prose-table">
          {error && <p className="text-danger text-sm">{error}</p>}
          {!error && !markdown && <p className="text-sm text-ink-muted">Generating...</p>}
          {markdown && <ReactMarkdown remarkPlugins={[remarkGfm]}>{markdown}</ReactMarkdown>}
        </div>
        <footer className="flex flex-wrap items-center gap-2 px-5 py-3 border-t border-line">
          <button
            type="button"
            disabled={!markdown}
            onClick={() => copy(markdown, 'Markdown copied.')}
            className="text-xs font-medium px-3 py-1.5 rounded-panel bg-accent text-white disabled:opacity-50"
          >
            Copy Markdown
          </button>
          <button
            type="button"
            disabled={!markdown}
            onClick={download}
            className="text-xs font-medium px-3 py-1.5 rounded-panel border border-line text-ink hover:bg-muted disabled:opacity-50"
          >
            Download .md
          </button>
          <button
            type="button"
            onClick={share}
            className="text-xs font-medium px-3 py-1.5 rounded-panel border border-line text-ink hover:bg-muted"
          >
            Share replay link
          </button>
          {status && <span className="text-xs text-success ml-auto">{status}</span>}
        </footer>
      </div>
    </div>
  );
}
