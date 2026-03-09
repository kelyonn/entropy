import ReactMarkdown from 'react-markdown';
import axios from 'axios';

const API_URL = import.meta.env.VITE_API_URL || '';

export default function AIPanel({ explanation, isAnalyzing, crashData, sessionId }) {
  const handleRestore = async () => {
    try {
      const res = await axios.get(`${API_URL}/api/session/restore?session_id=${sessionId}`);
      window.location.href = `/?session_id=${res.data.session_id}`;
    } catch (err) {
      alert('Restore failed: ' + err.message);
    }
  };

  return (
    <div className="h-full bg-gray-900 rounded-lg border border-gray-700 flex flex-col">
      {/* Header */}
      <div className="flex items-center gap-2 p-3 border-b border-gray-700">
        <span className="text-2xl">🤖</span>
        <span className="text-white font-semibold">Entropy AI</span>
        <span className="text-xs text-gray-400 ml-auto">
          powered by AWS Bedrock
        </span>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-4">
        {!crashData && !isAnalyzing && !explanation && (
          <div className="text-gray-500 text-sm text-center mt-8">
            <p className="text-3xl mb-3">💡</p>
            <p>Run a command in the terminal.</p>
            <p className="mt-1">When something crashes, I'll explain why.</p>
          </div>
        )}

        {isAnalyzing && (
          <div className="flex items-center gap-3 text-yellow-400">
            <div className="animate-spin text-xl">⚙️</div>
            <p className="text-sm">Analyzing crash with Claude 3.5 Sonnet...</p>
          </div>
        )}

        {explanation && !isAnalyzing && (
          <div className="prose prose-invert prose-sm max-w-none">
            <ReactMarkdown>{explanation}</ReactMarkdown>
          </div>
        )}
      </div>

      {/* Restore button */}
      {crashData && (
        <div className="p-3 border-t border-gray-700">
          <button
            onClick={handleRestore}
            className="w-full bg-blue-600 hover:bg-blue-700 text-white text-sm font-semibold
                       py-2 px-4 rounded-lg transition-colors"
          >
            🔄 Restore Session (2-3 sec)
          </button>
        </div>
      )}
    </div>
  );
}
