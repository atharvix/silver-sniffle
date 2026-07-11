import { Link } from 'wouter';
import '../landing.css';
import './legal.css';

interface Section {
  heading: string;
  body: string[];
}

interface Props {
  title: string;
  updated: string;
  intro: string;
  sections: Section[];
}

export default function LegalPage({ title, updated, intro, sections }: Props) {
  return (
    <div className="legal-page">
      <div className="legal-bg" />

      <nav className="navbar legal-navbar">
        <Link href="/" className="nav-logo legal-logo">s<span>_</span></Link>
        <Link href="/" className="legal-back">Back to home</Link>
      </nav>

      <main className="legal-main">
        <h1 className="legal-title">{title}</h1>
        <p className="legal-updated">Last updated {updated}</p>
        <p className="legal-intro">{intro}</p>

        {sections.map((section) => (
          <section className="legal-section" key={section.heading}>
            <h2>{section.heading}</h2>
            {section.body.map((paragraph, i) => (
              <p key={i}>{paragraph}</p>
            ))}
          </section>
        ))}
      </main>

      <footer className="footer legal-footer">
        <div className="footer-bottom">
          <Link href="/privacy" className="footer-link">Privacy</Link>
          <Link href="/terms-of-service" className="footer-link">Terms of Service</Link>
          <Link href="/terms-of-use" className="footer-link">Terms of Use</Link>
        </div>
      </footer>
    </div>
  );
}
