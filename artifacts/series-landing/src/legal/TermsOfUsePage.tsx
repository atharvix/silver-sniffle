import LegalPage from './LegalPage';

export default function TermsOfUsePage() {
  return (
    <LegalPage
      title="Terms of Use"
      updated="July 11, 2026"
      intro="These Terms of Use cover how you may use the Kinjo website and product, including content, branding, and intellectual property. They apply alongside our Terms of Service."
      sections={[
        {
          heading: '1. License to use Kinjo',
          body: [
            'We grant you a personal, non-transferable, non-exclusive license to use Kinjo on your own devices for its intended purpose: finding and connecting with people nearby. This license does not extend to any commercial or automated use of the product.',
          ],
        },
        {
          heading: '2. Ownership',
          body: [
            'The Kinjo name, logo, interface design, and all associated branding are the property of Kinjo and may not be copied, reproduced, or used without prior written permission.',
            'You retain ownership of the photos, name, and about text you add to your profile, but you grant us a limited license to display that content to other nearby users so the product can function.',
          ],
        },
        {
          heading: '3. Restrictions',
          body: [
            'You may not reverse-engineer, decompile, or attempt to extract the source code of Kinjo, use automated tools (bots, scrapers) to access the service, or resell, sublicense, or redistribute access to Kinjo.',
          ],
        },
        {
          heading: '4. Third-party press mentions',
          body: [
            'Any references to third-party publications on our site are used to indicate press coverage and do not imply endorsement of the publication by Kinjo or vice versa.',
          ],
        },
        {
          heading: '5. Termination',
          body: [
            'We may suspend your access to Kinjo at any time for conduct that violates these Terms of Use or our Terms of Service.',
          ],
        },
        {
          heading: '6. Contact',
          body: [
            'Questions about acceptable use or intellectual property can be sent to legal@seriesapp.com.',
          ],
        },
      ]}
    />
  );
}
