import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { setAuthTokenGetter } from '@workspace/api-client-react';

import App from './App';
import './index.css';

// Attach the bearer token from localStorage to every API call.
// (The generated client paths already include /api — no base URL override needed.)
setAuthTokenGetter(() => localStorage.getItem('series_token') ?? '');

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 30_000 },
    mutations: { retry: 0 },
  },
});

createRoot(document.getElementById('root')!).render(
  <QueryClientProvider client={queryClient}>
    <App />
  </QueryClientProvider>
);
