import { useRef, useState } from 'react';
import { Link } from 'wouter';
import './create-profile.css';

export default function CreateProfilePage() {
  const [photoUrl, setPhotoUrl] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setPhotoUrl(URL.createObjectURL(file));
  }

  return (
    <div className="cp-page">
      <div className="cp-bg" />

      <nav className="cp-navbar">
        <Link href="/" className="cp-nav-logo">Kinjo<span>_</span></Link>
        <div className="cp-nav-step">Step 1 of 2</div>
      </nav>

      <div className="cp-content">
        <div className="cp-card">
          <h1 className="cp-card-title">Create your profile</h1>
          <p className="cp-card-sub">
            This is what people nearby will see. Keep it real and keep it short.
          </p>

          {/* Photo upload */}
          <div className="cp-photo-row">
            <label className="cp-photo-upload" htmlFor="photo-input">
              {photoUrl ? (
                <img src={photoUrl} alt="Profile preview" />
              ) : (
                <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                  <path d="M12 15.2a4 4 0 100-8 4 4 0 000 8zm0-1.8a2.2 2.2 0 110-4.4 2.2 2.2 0 010 4.4zM9 2l-1.8 2H4a2 2 0 00-2 2v12a2 2 0 002 2h16a2 2 0 002-2V6a2 2 0 00-2-2h-3.2L15 2H9z" />
                </svg>
              )}
            </label>
            <input
              ref={inputRef}
              type="file"
              id="photo-input"
              accept="image/*"
              hidden
              onChange={handleFileChange}
            />
            <div className="cp-photo-info">
              <div className="cp-photo-label">Add a photo</div>
              <div className="cp-photo-hint">A clear shot of your face works best.</div>
            </div>
          </div>

          {/* Fields */}
          <div className="cp-field">
            <label htmlFor="cp-name">Name</label>
            <input type="text" id="cp-name" placeholder="Nischal Jain" />
          </div>

          <div className="cp-field">
            <label htmlFor="cp-doing">What you do</label>
            <input type="text" id="cp-doing" placeholder="Owner of a skincare brand" />
          </div>

          <div className="cp-field">
            <label htmlFor="cp-looking">What you're looking for</label>
            <textarea
              id="cp-looking"
              placeholder="Looking to expand my network, meet interesting people, and build a great brand."
            />
          </div>

          {/* Submit */}
          <button className="cp-submit-pill">
            <span className="cp-submit-dot">
              <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                <path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z" />
              </svg>
            </span>
            <span className="cp-submit-text">Continue</span>
          </button>
        </div>
      </div>
    </div>
  );
}
