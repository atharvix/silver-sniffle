import LegalPage from './LegalPage';

export default function TermsOfServicePage() {
  return (
    <LegalPage
      title="Terms of Service"
      updated="July 11, 2026"
      intro="These Terms of Service govern your access to and use of Series. By creating an account or using the product, you agree to these terms."
      sections={[
        {
          heading: '1. Eligibility',
          body: [
            'You must be at least 18 years old to use Series. By signing up, you confirm that you meet this requirement and that the email address you provide belongs to you.',
          ],
        },
        {
          heading: '2. Your account',
          body: [
            'Series accounts are verified by email using a one-time code — there are no passwords to manage. You are responsible for keeping access to your email secure, since it is the key to your account.',
            'You agree to provide accurate profile information and not to impersonate another person.',
          ],
        },
        {
          heading: '3. Acceptable use',
          body: [
            'Series is built for real, respectful connections between people who are actually nearby. You agree not to harass, threaten, or endanger other users, misrepresent your identity or location, or use the service for commercial solicitation, scraping, or spam.',
            'We may suspend or terminate accounts that violate these terms or that we reasonably believe put other users at risk.',
          ],
        },
        {
          heading: '4. Location sharing',
          body: [
            'The core of Series depends on sharing your approximate location while you are actively using the app so we can surface nearby people to you and you to them. You can stop this at any time by closing the app or signing out.',
          ],
        },
        {
          heading: '5. Availability',
          body: [
            'We aim to keep Series available and reliable, but the service is provided "as is" without guarantees of uninterrupted availability. Features may change, and nearby matches depend on other users being active in your area.',
          ],
        },
        {
          heading: '6. Limitation of liability',
          body: [
            'Series facilitates introductions between users but is not responsible for the conduct of any user, on or off the platform. Use good judgment when meeting anyone in person.',
          ],
        },
        {
          heading: '7. Changes to these terms',
          body: [
            'We may update these Terms of Service from time to time. Continued use of Series after changes take effect constitutes acceptance of the updated terms.',
          ],
        },
        {
          heading: '8. Contact',
          body: [
            'Questions about these terms can be sent to legal@seriesapp.com.',
          ],
        },
      ]}
    />
  );
}
