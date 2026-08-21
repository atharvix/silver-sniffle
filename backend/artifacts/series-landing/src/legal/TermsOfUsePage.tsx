import LegalPage from './LegalPage';

export default function TermsOfUsePage() {
  return (
    <LegalPage
      title="Terms of Use"
      updated="July 2026"
      intro={'These Terms of Use (\u201cTerms of Use\u201d) govern your access to and browsing of the website located at kinjo.world (the \u201cSite\u201d), operated by Kinjo (\u201cKinjo,\u201d \u201cwe,\u201d \u201cus,\u201d or \u201cour\u201d). They apply to anyone who visits the Site, regardless of whether you create a Kinjo account. By accessing or using the Site, you agree to these Terms of Use. If you do not agree, please do not use the Site.'}
      sections={[
        {
          heading: '1. Use of the Site',
          body: [
            'You may access and browse the Site for personal, non-commercial, informational purposes \u2014 for example, to learn about Kinjo, join a waitlist, read our policies, or contact us. You agree not to:',
            '\u2022 Copy, reproduce, republish, or redistribute any substantial portion of the Site\u2019s content without our prior written permission.',
            '\u2022 Use any automated system (bots, scrapers, crawlers) to access the Site in a manner that sends more requests than a human could reasonably produce, or to extract content in bulk.',
            '\u2022 Attempt to gain unauthorized access to any part of the Site, our systems, or any account.',
            '\u2022 Introduce viruses, malware, or other harmful code to the Site.',
            '\u2022 Use the Site in any way that violates applicable law or infringes the rights of others.',
            '\u2022 Frame or mirror any part of the Site without our permission.',
          ],
        },
        {
          heading: '2. Third-Party Links',
          body: [
            'The Site may contain links to third-party websites (for example, our social media pages or press coverage). These links are provided for convenience only. We do not control and are not responsible for the content, privacy practices, or terms of any third-party site, and inclusion of a link does not imply endorsement.',
          ],
        },
        {
          heading: '3. No Professional or Investment Advice',
          body: [
            'Any content on the Site \u2014 including descriptions of Kinjo\u2019s product, roadmap, or business \u2014 is provided for general informational purposes only and does not constitute professional, legal, financial, or investment advice. Statements about future features or plans are aspirational and may change without notice.',
          ],
        },
        {
          heading: '4. Demonstration Content',
          body: [
            'Any names, photos, profile descriptions, quotes, or similar content displayed on the Site to illustrate how Kinjo works (for example, in screenshots, mockups, sample profile cards, or product walkthroughs) are for demonstration purposes only. These depictions are fictional and/or AI-generated and do not represent real Kinjo users, real profile data, or real interactions. Any resemblance to an actual person is coincidental. Demonstration content should not be relied upon as an example of any specific real user\u2019s information or as a guarantee of the exact content, accuracy, or output you will see when using the actual Service.',
          ],
        },
        {
          heading: '5. Limitation of Liability',
          body: [
            'TO THE MAXIMUM EXTENT PERMITTED BY LAW, KINJO WILL NOT BE LIABLE FOR ANY INDIRECT, INCIDENTAL, SPECIAL, CONSEQUENTIAL, OR PUNITIVE DAMAGES ARISING FROM YOUR USE OF, OR INABILITY TO USE, THE SITE, EVEN IF WE HAVE BEEN ADVISED OF THE POSSIBILITY OF SUCH DAMAGES.',
          ],
        },
        {
          heading: '6. Changes to These Terms of Use',
          body: [
            'We may revise these Terms of Use from time to time by posting an updated version on the Site. Material changes will be indicated by an updated \u201cLast updated\u201d date.',
          ],
        },
        {
          heading: '7. Severability',
          body: [
            'If any provision of these Terms of Use is found unenforceable, the remaining provisions will remain in full effect.',
          ],
        },
        {
          heading: '8. Contact',
          body: [
            'Questions about these Terms of Use can be sent to: hello@kinjo.world',
          ],
        },
      ]}
    />
  );
}
