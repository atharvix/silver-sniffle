import LegalPage from './LegalPage';

export default function PrivacyPage() {
  return (
    <LegalPage
      title="Privacy Policy"
      updated="July 2026"
      intro={'This Privacy Policy explains how Kinjo (\u201cKinjo,\u201d \u201cwe,\u201d \u201cus,\u201d or \u201cour\u201d) collects, uses, shares, and protects information in connection with the Kinjo mobile application, website, and related services (the \u201cService\u201d). By using the Service, you agree to the collection and use of information as described in this Policy. If you do not agree, please do not use the Service.'}
      sections={[
        {
          heading: '1. Information We Collect',
          body: [
            'a. Information you provide directly \u2014 Email ID (for account creation and OTP verification); profile information you enter or edit (name, photo, professional details, self-authored prompt/quote text); communications you send to us (e.g., support requests, reports about other users).',
            'b. Location information \u2014 When you enable \u201cvisibility,\u201d we collect your device\u2019s location on an ongoing basis while visibility is active, in order to detect proximity to other visible users. We do not collect your location while your visibility is turned off, except as strictly necessary for core app functionality (e.g., determining your general region for setup purposes), if applicable.',
            'c. AI-processed information \u2014 Information you or your connected account provide may be processed by AI systems (our own or third-party providers) to generate profile summaries, conversation-starter suggestions, and topic tags.',
            'd. Device and usage information \u2014 Device identifiers, operating system, app version, crash logs, and general usage/interaction data (e.g., how many profile cards you\u2019ve received), collected automatically to operate and improve the Service.',
            'e. Photos \u2014 Profile photos you upload are used solely to display your profile card to nearby visible users and are not used for facial recognition or biometric identification by Kinjo.',
          ],
        },
        {
          heading: '2. How We Use Information',
          body: [
            'We use the information above to:',
            '\u2022 Create and maintain your account and verify your identity via OTP.',
            '\u2022 Generate your profile card content, including AI-generated summaries and conversation starters.',
            '\u2022 Detect proximity between visible users and trigger the exchange of profile cards.',
            '\u2022 Provide customer support and respond to reports of misconduct.',
            '\u2022 Maintain the security, integrity, and reliability of the Service.',
            '\u2022 Improve and develop the Service, including improving the quality of AI-generated content and proximity detection.',
            'We do not use your location data for advertising or sell your location data to third parties.',
          ],
        },
        {
          heading: '3. How Information Is Shared With Other Users',
          body: [
            'Kinjo\u2019s core function involves sharing certain information with other users when a proximity event occurs. Specifically, when you are visible and come within range of another visible user, both of you will receive a profile card that may include:',
            '\u2022 Your name and photo.',
            '\u2022 An AI-generated summary and conversation starters.',
            '\u2022 Professional/expertise information drawn from your profile or connected account.',
            '\u2022 Contextual information such as the event or venue you\u2019re associated with, if applicable.',
            'We do not share your continuous, live location with other users \u2014 only the fact of proximity, expressed through the profile-card exchange. We do not provide direct messaging between users, and we do not share your phone number, exact address, or precise coordinates with other users.',
          ],
        },
        {
          heading: '4. Third-Party Service Providers',
          body: [
            'We share information with third-party vendors who help us operate the Service, including:',
            '\u2022 SMS/OTP verification providers, to deliver and validate one-time passcodes.',
            '\u2022 AI/language-model providers, to generate profile summaries and conversation suggestions.',
            '\u2022 Cloud hosting and infrastructure providers, to store and process data.',
            '\u2022 Analytics and crash-reporting providers, to help us understand and improve app performance.',
            'These providers are authorized to use your information only as necessary to provide their services to us and are bound by confidentiality and data-protection obligations. A current list of categories of sub-processors is available on request at hello@kinjo.world.',
          ],
        },
        {
          heading: '5. Location Data \u2014 Additional Detail',
          body: [
            'Because location sharing enables the core function of the Service, we want to be specific:',
            '\u2022 Location sharing is off by default and requires your affirmative action to enable (\u201cgo visible\u201d).',
            '\u2022 You can disable visibility at any time; doing so stops new location collection for proximity purposes going forward.',
            '\u2022 Depending on your device settings, you may also need to grant OS-level location permissions; you can manage or revoke these at any time in your device settings, which will also disable Kinjo\u2019s ability to detect proximity.',
          ],
        },
        {
          heading: '6. Children\u2019s Privacy',
          body: [
            'The Service is not directed to, and may not be used by, anyone under the age of 18. We do not knowingly collect personal information from minors. If we learn we have collected information from someone under 18, we will delete it promptly. If you believe a minor has provided us information, contact us at hello@kinjo.world.',
          ],
        },
        {
          heading: '7. Data Security',
          body: [
            'We use technical and physical safeguards designed to protect your information, including in transit, access controls, and regular security review of our systems and vendors. No system is completely secure, and we cannot guarantee absolute security of your information.',
          ],
        },
        {
          heading: '8. Changes to This Policy',
          body: [
            'We may update this Privacy Policy from time to time. If we make material changes, we will notify you through the app, our website, or by other reasonable means before the changes take effect.',
          ],
        },
        {
          heading: '9. Contact Us',
          body: [
            'If you have questions about this Privacy Policy or how we handle your information, contact us at: hello@kinjo.world',
          ],
        },
      ]}
    />
  );
}
