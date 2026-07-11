import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import EmailVerificationModal from './EmailVerificationModal';

function renderModal(props: Partial<React.ComponentProps<typeof EmailVerificationModal>> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const onDiscovery = vi.fn();
  const onClose = vi.fn();
  render(
    <QueryClientProvider client={queryClient}>
      <EmailVerificationModal onClose={onClose} onDiscovery={onDiscovery} {...props} />
    </QueryClientProvider>
  );
  return { onDiscovery, onClose };
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

describe('EmailVerificationModal — OTP verification', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  it('stores the verification token in localStorage and advances to the profile step on success', async () => {
    const user = userEvent.setup();
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/auth/send-otp')) {
        return jsonResponse({ devOtp: '1234' });
      }
      if (url.includes('/api/auth/verify-otp')) {
        return jsonResponse({ verificationToken: 'tok_abc123' });
      }
      throw new Error(`Unexpected fetch: ${url}`);
    });

    renderModal();

    await user.type(screen.getByPlaceholderText('you@example.com'), 'person@example.com');
    await user.click(screen.getByRole('button', { name: /send code/i }));

    // Land on OTP step
    await screen.findByText(/enter the code/i);

    const otpInputs = screen.getAllByRole('textbox').filter(el => (el as HTMLInputElement).maxLength === 1);
    for (let i = 0; i < 4; i++) {
      fireEvent.change(otpInputs[i], { target: { value: String(i + 1) } });
    }

    await waitFor(() => {
      expect(localStorage.getItem('series_token')).toBe('tok_abc123');
    });
    expect(localStorage.getItem('series_email')).toBe('person@example.com');

    // Advances past OTP to the profile step
    await screen.findByText(/set up your profile/i);
  });

  it('shows an error and does not store a token when verification fails', async () => {
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/auth/verify-otp')) {
        return jsonResponse({ error: 'Invalid code' }, 400);
      }
      throw new Error(`Unexpected fetch: ${url}`);
    });

    renderModal({ initialStep: 'otp' });

    const otpInputs = screen.getAllByRole('textbox').filter(el => (el as HTMLInputElement).maxLength === 1);
    for (let i = 0; i < 4; i++) {
      fireEvent.change(otpInputs[i], { target: { value: String(i + 1) } });
    }

    await screen.findByText('Invalid code');
    expect(localStorage.getItem('series_token')).toBeNull();
  });
});

describe('EmailVerificationModal — profile submission', () => {
  beforeEach(() => {
    localStorage.setItem('series_token', 'tok_existing');
    localStorage.setItem('series_email', 'person@example.com');
    vi.stubGlobal('fetch', vi.fn());
  });

  it('advances to the location step when the profile save succeeds', async () => {
    const user = userEvent.setup();
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/profiles/location')) throw new Error('should not be called yet');
      if (url.includes('/api/auth/send-welcome')) return jsonResponse({ sent: true });
      if (url.includes('/api/profiles')) return jsonResponse({ id: 'p1', name: 'Alex' });
      throw new Error(`Unexpected fetch: ${url}`);
    });

    renderModal({ initialStep: 'profile' });

    await user.type(screen.getByPlaceholderText('Your full name'), 'Alex');
    await user.click(screen.getByRole('button', { name: /continue/i }));

    await screen.findByText(/find people nearby/i);
    expect(localStorage.getItem('series_has_profile')).toBe('true');
  });

  it('shows a save error and stays on the profile step on a 500', async () => {
    const user = userEvent.setup();
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/profiles')) return jsonResponse({ error: 'Server error' }, 500);
      throw new Error(`Unexpected fetch: ${url}`);
    });

    renderModal({ initialStep: 'profile' });

    await user.type(screen.getByPlaceholderText('Your full name'), 'Alex');
    await user.click(screen.getByRole('button', { name: /continue/i }));

    await screen.findByText('Could not save profile. Please try again.');
    // Still on the profile step, not silently stuck with no feedback.
    expect(screen.getByPlaceholderText('Your full name')).toBeInTheDocument();
  });

  it('clears the session and returns to the email step on a 401', async () => {
    const user = userEvent.setup();
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/profiles')) return jsonResponse({ error: 'Unauthorized' }, 401);
      throw new Error(`Unexpected fetch: ${url}`);
    });

    renderModal({ initialStep: 'profile' });

    await user.type(screen.getByPlaceholderText('Your full name'), 'Alex');
    await user.click(screen.getByRole('button', { name: /continue/i }));

    await screen.findByText('Session expired. Please verify your email again.');
    expect(localStorage.getItem('series_token')).toBeNull();
    // Back at step 1
    expect(screen.getByPlaceholderText('you@example.com')).toBeInTheDocument();
  });
});

describe('EmailVerificationModal — location submission', () => {
  beforeEach(() => {
    localStorage.setItem('series_token', 'tok_existing');
    localStorage.setItem('series_email', 'person@example.com');
    localStorage.setItem('series_has_profile', 'true');
    vi.stubGlobal('fetch', vi.fn());
    vi.stubGlobal('navigator', {
      ...navigator,
      geolocation: {
        getCurrentPosition: vi.fn((success: PositionCallback) => {
          success({
            coords: { latitude: 37.7749, longitude: -122.4194 } as GeolocationCoordinates,
          } as GeolocationPosition);
        }),
      },
    });
  });

  it('sends the coordinates to the API and transitions to discovery on success', async () => {
    const user = userEvent.setup();
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/profiles/location')) return jsonResponse({ ok: true });
      throw new Error(`Unexpected fetch: ${url}`);
    });

    const { onDiscovery } = renderModal({ initialStep: 'location' });

    await user.click(screen.getByRole('button', { name: /allow location access/i }));

    await waitFor(() => expect(onDiscovery).toHaveBeenCalledTimes(1));

    const call = fetchMock.mock.calls.find(([input]) => String(input).includes('/api/profiles/location'));
    expect(call).toBeTruthy();
    const body = JSON.parse((call![1] as RequestInit).body as string);
    expect(body).toEqual({ latitude: 37.7749, longitude: -122.4194 });
  });

  it('shows an error and does not navigate when the location save fails', async () => {
    const user = userEvent.setup();
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/profiles/location')) return jsonResponse({ error: 'Server error' }, 500);
      throw new Error(`Unexpected fetch: ${url}`);
    });

    const { onDiscovery } = renderModal({ initialStep: 'location' });

    await user.click(screen.getByRole('button', { name: /allow location access/i }));

    await screen.findByText('Could not share location. Please try again.');
    expect(onDiscovery).not.toHaveBeenCalled();
  });

  it('silently re-verifies on an expired token during location save, then returns to location (not profile) after OTP', async () => {
    const user = userEvent.setup();
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/profiles/location')) return jsonResponse({ error: 'Unauthorized' }, 401);
      if (url.includes('/api/auth/send-otp')) return jsonResponse({ devOtp: '5678' });
      if (url.includes('/api/auth/verify-otp')) return jsonResponse({ verificationToken: 'tok_fresh' });
      throw new Error(`Unexpected fetch: ${url}`);
    });

    const { onDiscovery } = renderModal({ initialStep: 'location' });

    await user.click(screen.getByRole('button', { name: /allow location access/i }));

    // Expired token triggers a silent re-send and lands on the OTP step
    // with re-verify copy — not the profile step, and not step 1 (email).
    await screen.findByText(/re-verify your email/i);
    expect(screen.getByText(/your profile is already saved/i)).toBeInTheDocument();
    expect(localStorage.getItem('series_token')).toBeNull();
    // Email + profile-completion flag are preserved, not wiped.
    expect(localStorage.getItem('series_email')).toBe('person@example.com');
    expect(localStorage.getItem('series_has_profile')).toBe('true');

    const otpInputs = screen.getAllByRole('textbox').filter(el => (el as HTMLInputElement).maxLength === 1);
    for (let i = 0; i < 4; i++) {
      fireEvent.change(otpInputs[i], { target: { value: String(i + 1) } });
    }

    // Completing the OTP stores the fresh token and returns to the
    // location step, skipping the profile step entirely.
    await waitFor(() => {
      expect(localStorage.getItem('series_token')).toBe('tok_fresh');
    });
    await screen.findByRole('button', { name: /allow location access/i });
    expect(screen.queryByPlaceholderText('Your full name')).not.toBeInTheDocument();
    expect(onDiscovery).not.toHaveBeenCalled();
  });
});
