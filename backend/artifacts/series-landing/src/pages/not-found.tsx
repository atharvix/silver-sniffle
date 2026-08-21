import { Link } from 'wouter';

export default function NotFound() {
  return (
    <div style={{
      minHeight: '100vh',
      background: 'linear-gradient(160deg, #1e1408 0%, #170f06 40%, #120b04 72%, #0e0b08 100%)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, sans-serif",
      color: '#fff',
      padding: 24,
    }}>
      <div style={{ textAlign: 'center', maxWidth: 400 }}>
        <p style={{
          fontSize: 13,
          fontWeight: 700,
          letterSpacing: '0.12em',
          textTransform: 'uppercase',
          color: '#e8a838',
          marginBottom: 20,
        }}>
          404
        </p>
        <h1 style={{
          fontSize: 'clamp(38px, 9vw, 60px)',
          fontWeight: 900,
          letterSpacing: '-2px',
          lineHeight: 1.0,
          marginBottom: 18,
        }}>
          Page not found
        </h1>
        <p style={{
          fontSize: 15,
          color: 'rgba(255,255,255,0.55)',
          lineHeight: 1.65,
          marginBottom: 40,
        }}>
          This page doesn't exist. Head back and find people around you.
        </p>
        <Link href="/" style={{
          display: 'inline-block',
          background: '#fff',
          color: '#111',
          borderRadius: 12,
          padding: '14px 36px',
          fontSize: 15,
          fontWeight: 700,
          textDecoration: 'none',
          letterSpacing: '-0.2px',
        }}>
          Back to Kinjo
        </Link>
      </div>
    </div>
  );
}
