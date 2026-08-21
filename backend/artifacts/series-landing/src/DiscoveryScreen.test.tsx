import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import DiscoveryScreen from './DiscoveryScreen';

function renderScreen() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={queryClient}>
      <DiscoveryScreen onBack={vi.fn()} />
    </QueryClientProvider>
  );
  return queryClient;
}

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Error',
    headers: new Headers({ 'content-type': 'application/json' }),
    json: async () => body,
    text: async () => JSON.stringify(body),
  } as Response;
}

describe('DiscoveryScreen', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  it('shows a loading skeleton before the API responds, then renders profile cards', async () => {
    let resolveFetch!: (value: Response) => void;
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockImplementation(
      () => new Promise<Response>(resolve => { resolveFetch = resolve; })
    );

    renderScreen();

    // Skeleton is visible while the request is in flight.
    expect(screen.getAllByRole('generic').length).toBeGreaterThan(0);
    expect(screen.queryByText(/no one nearby yet/i)).not.toBeInTheDocument();

    resolveFetch(jsonResponse({
      profiles: [
        { name: 'Jamie Lee', photo: null, distanceMeters: 12, conversationStarter: 'Hi there!' },
      ],
    }));

    await screen.findByText('Jamie Lee');
    expect(screen.getByText(/1 person within 30 m/i)).toBeInTheDocument();
  });

  it('shows an empty state when the API returns no nearby profiles', async () => {
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockResolvedValue(jsonResponse({ profiles: [] }));

    renderScreen();

    await screen.findByText(/no one nearby yet/i);
  });

  it('shows an error state when the API call fails', async () => {
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockResolvedValue(jsonResponse({ error: 'Server error' }, 500));

    renderScreen();

    await screen.findByText(/couldn't load profiles/i);
  });
});
