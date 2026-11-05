import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import 'highlight.js/styles/github.css';

function SubjectLine({ subject }) {
  if (!subject) return null;
  if (subject.question && subject.question !== 'Explain more simply') {
    return <span className="truncate">Question: {subject.question}</span>;
  }
  const cmd = (subject.command || '').split('\n')[0];
  return (
    <span className="truncate">
      {subject.exitCode ? `exit ${subject.exitCode} from ` : ''}
      <code className="font-mono">{cmd}</code>
      {subject.question === 'Explain more simply' ? ', simpler' : ''}
    </span>
  );
}

export default function AIPanel({
  text,
  subject,
  history = [],
  isAnalyzing,
  aiFallback,
  lastFailure,
  connected,
  expanded,
  onToggle,
  onAsk,
  onExplainFailure,
}) {
  const [question, setQuestion] = useState('');
  const [openHistory, setOpenHistory] = useState(null);
  const bodyRef = useRef(null);

  useEffect(() => {
    if (isAnalyzing && bodyRef.current) {
      bodyRef.current.scrollTop = bodyRef.current.scrollHeight;
    }
  }, [text, isAnalyzing]);

  const submit = (e) => {
    e.preventDefault();
    const q = question.trim();
    if (!q || isAnalyzing) return;
    onAsk(q, false);
    setQuestion('');
  };

  const previous = history.filter((h) => h.text !== text).slice().reverse();
  const canExplainFailure =
    lastFailure &&
    !isAnalyzing &&
    !(subject && subject.command === lastFailure.command && !subject.question);

  return (
    <section className="panel flex flex-col">
      <button
        type="button"
        onClick={onToggle}
        className="flex items-center justify-between gap-3 px-4 py-3 text-left w-full"
        aria-expanded={expanded}
      >
        <div className="min-w-0">
          <div className="text-sm font-semibold text-ink">Explanation</div>
          <div className="text-xs text-ink-muted flex gap-1 min-w-0">
            {isAnalyzing ? (
              <span className="flex items-center gap-2 text-warning">
                <span className="spinner" /> Analyzing
              </span>
            ) : subject ? (
              <SubjectLine subject={subject} />
            ) : (
              <span>Root cause, impact, and fix for failures in this session</span>
            )}
          </div>
        </div>
        <span className="text-xs text-ink-muted shrink-0">{expanded ? 'Collapse' : 'Expand'}</span>
      </button>

      <div className="drawer-panel border-t border-line" style={{ maxHeight: expanded ? '560px' : '0px', opacity: expanded ? 1 : 0 }}>
        {aiFallback && (
          <div className="px-4 py-2 text-xs text-warning border-b border-line bg-muted">
            The AI service is unavailable. Showing a prepared explanation.
          </div>
        )}

        <div ref={bodyRef} className="px-4 py-3 overflow-y-auto" style={{ maxHeight: '340px' }}>
          {!text && !isAnalyzing && (
            <div className="text-sm text-ink-muted space-y-1">
              <p>Every failed command is explained here automatically, using what you ran just before it.</p>
              <p>During a lab, explanations point at your mistake without giving away the fix. Ask anything below.</p>
            </div>
          )}
          {text && (
            <div className="prose-entropy">
              <ReactMarkdown rehypePlugins={[rehypeHighlight]}>{text}</ReactMarkdown>
            </div>
          )}
          {isAnalyzing && !text && <p className="text-sm text-ink-muted">Waiting for the first tokens...</p>}

          {previous.length > 0 && (
            <div className="mt-4 pt-3 border-t border-line">
              <div className="text-xs font-medium text-ink-muted mb-1">Earlier in this session</div>
              <ul className="divide-y divide-line">
                {previous.map((h) => (
                  <li key={h.id} className="py-1.5">
                    <button
                      type="button"
                      onClick={() => setOpenHistory(openHistory === h.id ? null : h.id)}
                      className="w-full text-left text-xs text-ink hover:text-accent flex gap-1 min-w-0"
                    >
                      <SubjectLine subject={h.subject || { command: 'explanation' }} />
                    </button>
                    {openHistory === h.id && (
                      <div className="prose-entropy mt-2">
                        <ReactMarkdown rehypePlugins={[rehypeHighlight]}>{h.text}</ReactMarkdown>
                      </div>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>

        <div className="px-4 py-3 border-t border-line space-y-2">
          <div className="flex flex-wrap gap-2">
            {canExplainFailure && (
              <button
                type="button"
                onClick={onExplainFailure}
                disabled={!connected}
                className="text-xs font-medium px-3 py-1.5 rounded-panel border border-line text-ink hover:bg-muted disabled:opacity-50 max-w-full truncate"
              >
                Explain exit {lastFailure.exitCode} from <code className="font-mono">{lastFailure.command.split('\n')[0].slice(0, 40)}</code>
              </button>
            )}
            {text && !isAnalyzing && (
              <button
                type="button"
                onClick={() => onAsk('', true)}
                disabled={!connected}
                className="text-xs font-medium px-3 py-1.5 rounded-panel border border-line text-ink hover:bg-muted disabled:opacity-50"
              >
                Explain more simply
              </button>
            )}
          </div>
          <form onSubmit={submit} className="flex gap-2">
            <input
              type="text"
              value={question}
              onChange={(e) => setQuestion(e.target.value)}
              placeholder="Ask about a command, a signal, a kernel limit..."
              disabled={isAnalyzing || !connected}
              className="flex-1 min-w-0 text-sm px-3 py-1.5 rounded-panel border border-line bg-surface text-ink placeholder:text-ink-muted focus:outline-none focus:border-accent"
            />
            <button
              type="submit"
              disabled={isAnalyzing || !question.trim() || !connected}
              className="text-xs font-medium px-3 py-1.5 rounded-panel bg-accent text-white disabled:opacity-50"
            >
              Ask
            </button>
          </form>
        </div>
      </div>
    </section>
  );
}
