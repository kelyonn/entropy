import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import ReplayView from './components/ReplayView';
import './index.css';

const params = new URLSearchParams(window.location.search);
const replayToken = params.get('replay');

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>{replayToken ? <ReplayView token={replayToken} labId={params.get('lab')} /> : <App />}</React.StrictMode>,
);
