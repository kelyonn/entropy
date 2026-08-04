import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import 'highlight.js/styles/github.css';
import axios from 'axios';

const API_URL = import.meta.env.VITE_API_URL || '';

export default function AIPanel({
  explanation,
  history = [],
  isAnalyzing,
  crashData,
  sessionId,
  expanded,
  onToggle,
  toast,
  onAskAI,
  aiFallback,
}) {
  const [question, setQuestion] = useState('');

  const handleRestore = async () => {
    try {
      const res = await axios.get(`${API_URL}/api/session/restore?session_id=${sessionId}`);
      window.location.href = `/?session_id=${res.data.session_id}`;
    } catch (err) {
      alert('Restore failed: ' + err.message);
    }
  };

  const handleAsk = (e) => {
    e.preventDefault();
    if (!question.trim() || isAnalyzing) return;
    onAskAI(question.trim(), false);
    setQuestion('');
  };

  return (
    <div className="panel flex flex-col">
      <button
        type="button"
        onClick={onToggle}
        className="flex items-center justify-between px-4 py-3 border-b border-line text-left w-full"
      >
        <div>
          <div className="text-sm font-semibold text-ink">Explanation</div>
          <div className="text-xs text-ink-muted">Plain-language breakdown of what went wrong</div>
        </div>
        <span className="text-xs text-ink-muted">{expanded ? 'Collapse' : 'Expand'}</span>
      </button>

      <div
        className="drawer-panel"
        style={{ maxHeight: expanded ? '420px' : '0px', opacity: expanded ? 1 : 0 }}
      >
        {aiFallback && (
          <div className="px-4 py-2 text-xs text-warning border-b border-line bg-muted">
            AI service unavailable. Showing fallback explanation.
          </div>
        )}

        <div className="px-4 py-3 overflow-y-auto" style={{ maxHeight: '280px' }}>
          {!crashData && !isAnalyzing && !explanation && history.length === 0 && (
            <div className="text-sm text-ink-muted">
              <p>Run a command in the terminal.</p>
              <p className="mt-1">When something fails, a plain-language explanation appears here.</p>
            </div>
          )}

          {history.map((item, i) => (
            <div key={i} className="mb-3 pb-3 border-b border-line prose-entropy opacity-70">
              <div className="text-xs text-ink-muted mb-1">Previous explanation</div>
              <ReactMarkdown rehypePlugins={[rehypeHighlight]}>{item}</ReactMarkdown>
            </div>
          ))}

          {isAnalyzing && (
            <div className="flex items-center gap-2 text-warning text-sm">
              <div className="spinner" />
              <span>Analyzing failure...</span>
            </div>
          )}

          {explanation && (
            <div className="prose-entropy">
              <ReactMarkdown rehypePlugins={[rehypeHighlight]}>{explanation}</ReactMarkdown>
            </div>
          )}
        </div>

        {crashData && (
          <div className="px-4 py-3 border-t border-line space-y-2">
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => onAskAI('', true)}
                disabled={isAnalyzing}
                className="text-xs font-medium px-3 py-1.5 rounded-panel border border-line text-ink hover:bg-muted disabled:opacity-50"
              >
                Explain more simply
              </button>
            </div>
            <form onSubmit={handleAsk} className="flex gap-2">
              <input
                type="text"
                value={question}
                onChange={(e) => setQuestion(e.target.value)}
                placeholder="Ask a follow-up question..."
                disabled={isAnalyzing}
                className="flex-1 text-sm px-3 py-1.5 rounded-panel border border-line bg-surface text-ink placeholder:text-ink-muted"
              />
              <button
                type="submit"
                disabled={isAnalyzing || !question.trim()}
                className="text-xs font-medium px-3 py-1.5 rounded-panel bg-accent text-white disabled:opacity-50"
              >
                Ask
              </button>
            </form>
            <button
              type="button"
              onClick={handleRestore}
              className="w-full bg-accent hover:opacity-90 text-white text-sm font-medium py-2 px-4 rounded-panel transition-opacity"
            >
              Restore session
            </button>
          </div>
        )}
      </div>

      {toast && (
        <div className="px-4 py-2 text-xs text-success border-t border-line bg-accent-soft">
          {toast}
        </div>
      )}
    </div>
  );
}
