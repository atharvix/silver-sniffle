import { useState } from 'react';
import { Link } from 'wouter';
import './landing.css';
import EmailVerificationModal from './EmailVerificationModal';
import EditProfileModal from './EditProfileModal';
import AccountMenu from './AccountMenu';
import { useGetMyProfile, getGetMyProfileQueryKey } from '@workspace/api-client-react';

interface Props {
  onDiscovery: () => void;
}

export default function LandingPage({ onDiscovery }: Props) {
  const [showModal, setShowModal] = useState(false);
  const [initialStep, setInitialStep] = useState<'email' | 'location' | 'reverify'>('email');
  const [showEditProfile, setShowEditProfile] = useState(false);
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('series_token'));

  const myProfile = useGetMyProfile({
    query: { queryKey: getGetMyProfileQueryKey(), enabled: !!token, retry: false },
  });
  const isLoggedIn = !!token && !!myProfile.data;

  function handleStartConnecting() {
    const hasProfile = localStorage.getItem('series_has_profile') === 'true';
    const savedEmail = localStorage.getItem('series_email');

    if (token && hasProfile) {
      // Active session + profile — skip straight to location sharing.
      setInitialStep('location');
    } else if (!token && savedEmail && hasProfile) {
      // Token expired but we know this user's email and they already set up
      // their profile — auto-send a fresh OTP and jump to the OTP box.
      // No email re-entry, no profile re-fill.
      setInitialStep('reverify');
    } else {
      setInitialStep('email');
    }

    setShowModal(true);
  }

  function handleLogin() {
    const hasProfile = localStorage.getItem('series_has_profile') === 'true';
    const savedEmail = localStorage.getItem('series_email');

    if (!token && savedEmail && hasProfile) {
      // Returning user whose session lapsed — auto-send OTP to known email,
      // skip straight to the 4-digit box and then to the discovery screen.
      setInitialStep('reverify');
    } else {
      setInitialStep('email');
    }

    setShowModal(true);
  }

  function handleSignOut() {
    const signedOutToken = token;
    localStorage.removeItem('series_token');
    localStorage.removeItem('series_email');
    localStorage.removeItem('series_has_profile');
    setToken(null);

    // Best-effort: mark the profile offline so it drops out of nearby
    // results immediately instead of waiting for the heartbeat to expire.
    if (signedOutToken) {
      fetch('/api/profiles/offline', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: signedOutToken }),
      }).catch(() => {});
    }
  }

  return (
    <>
      {/* HERO */}
      <section className="hero">
        <div className="hero-bg" />

        <nav className="navbar">
          <div className="nav-logo">s<span>_</span></div>
          <div className="nav-btns">
            {isLoggedIn && myProfile.data ? (
              <AccountMenu
                name={myProfile.data.name}
                email={myProfile.data.email}
                photo={myProfile.data.photo || null}
                onEditProfile={() => setShowEditProfile(true)}
                onSignOut={handleSignOut}
              />
            ) : (
              <>
                <button className="btn-login" onClick={handleLogin}>Login</button>
                <button className="btn-signup" onClick={handleStartConnecting}>Sign up</button>
              </>
            )}
          </div>
        </nav>

        <div className="main-row">
          <h1 className="hero-headline">See who's<br />around you</h1>

          <div className="hero-right">
            <div className="cta-pill" onClick={handleStartConnecting}>
              <div className="cta-dot">
                <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                  <path d="M12 2C6.48 2 2 6.48 2 12c0 1.85.5 3.58 1.37 5.06L2 22l4.94-1.37C8.42 21.5 10.15 22 12 22c5.52 0 10-4.48 10-10S17.52 2 12 2z" />
                </svg>
              </div>
              <span className="cta-text">Start Looking</span>
            </div>
            <p className="cta-subtext">Not another social app.<br />The people here are actually near you.</p>
          </div>
        </div>
      </section>

      {/* FOOTER */}
      <footer className="footer">
        <div className="footer-top">
          <div className="footer-logo">Series</div>
        </div>

        <div className="footer-bottom">
          <Link href="/privacy" className="footer-link">Privacy</Link>
          <Link href="/terms-of-service" className="footer-link">Terms of Service</Link>
          <Link href="/terms-of-use" className="footer-link">Terms of Use</Link>
        </div>
      </footer>

      {showModal && (
        <EmailVerificationModal
          initialStep={initialStep}
          onClose={() => {
            // Even if the user bails out before sharing location, the email
            // step may already have issued a valid token (e.g. an existing
            // user who just re-verified) — re-sync so the navbar reflects
            // the signed-in state instead of sticking on Login/Sign up.
            setShowModal(false);
            setToken(localStorage.getItem('series_token'));
          }}
          onDiscovery={() => {
            setShowModal(false);
            setToken(localStorage.getItem('series_token'));
            onDiscovery();
          }}
        />
      )}

      {showEditProfile && myProfile.data && (
        <EditProfileModal
          initialName={myProfile.data.name}
          initialAbout={myProfile.data.about}
          initialPhoto={myProfile.data.photo || null}
          onClose={() => setShowEditProfile(false)}
          onSaved={() => {
            setShowEditProfile(false);
            myProfile.refetch();
          }}
        />
      )}
    </>
  );
}
