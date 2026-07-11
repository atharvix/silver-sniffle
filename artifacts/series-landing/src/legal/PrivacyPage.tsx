import LegalPage from './LegalPage';

export default function PrivacyPage() {
  return (
    <LegalPage
      title="Privacy Policy"
      updated="July 11, 2026"
      intro="Series helps you find and message people who are physically near you, right inside iMessage. This policy explains what information we collect, why we collect it, and how you can control it."
      sections={[
        {
          heading: '1. Information we collect',
          body: [
            'Account information: your email address, and any name, photo, and short "about" description you choose to add to your profile.',
            'Location information: while you have Series open and are actively looking, we collect your approximate device location so we can show you people nearby and show them you. We do not collect location in the background when the app is closed.',
            'Usage information: basic technical data such as device type and connection timestamps, used to keep the nearby feed accurate and to detect abuse.',
          ],
        },
        {
          heading: '2. How we use your information',
          body: [
            'To operate the core feature of the product: showing you other Series users who are currently nearby, and showing your profile to them.',
            'To verify your identity via one-time email codes, so accounts stay tied to a real, reachable email address.',
            'To keep the service safe — this includes detecting spam, fake accounts, and abusive behavior.',
          ],
        },
        {
          heading: '3. What we share',
          body: [
            'Your name, photo, and about text are visible to other Series users who are nearby at the same time as you. Your exact location is never shown to other users — only relative proximity.',
            'We do not sell your personal information. We share data with service providers only as needed to run the product (for example, sending verification emails), and they are not permitted to use it for their own purposes.',
          ],
        },
        {
          heading: '4. Your controls',
          body: [
            'You can edit or remove your name, photo, and about text at any time from your account menu.',
            'Closing the app or signing out stops your location from being shared and removes you from other people\'s nearby feed.',
            'You can request deletion of your account and associated data by contacting us.',
          ],
        },
        {
          heading: '5. Data retention',
          body: [
            'We keep your profile information for as long as your account is active. Location data used to power the live nearby feed is short-lived and is not retained once you go offline.',
          ],
        },
        {
          heading: '6. Contact',
          body: [
            'Questions about this policy or your data can be sent to privacy@seriesapp.com.',
          ],
        },
      ]}
    />
  );
}
