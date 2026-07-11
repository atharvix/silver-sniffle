import { useState } from 'react';
import { Switch, Route } from 'wouter';
import LandingPage from './LandingPage';
import DiscoveryScreen from './DiscoveryScreen';
import PrivacyPage from './legal/PrivacyPage';
import TermsOfServicePage from './legal/TermsOfServicePage';
import TermsOfUsePage from './legal/TermsOfUsePage';

function Home() {
  const [view, setView] = useState<'landing' | 'discovery'>('landing');

  if (view === 'discovery') {
    return <DiscoveryScreen onBack={() => setView('landing')} />;
  }

  return <LandingPage onDiscovery={() => setView('discovery')} />;
}

export default function App() {
  return (
    <Switch>
      <Route path="/privacy" component={PrivacyPage} />
      <Route path="/terms-of-service" component={TermsOfServicePage} />
      <Route path="/terms-of-use" component={TermsOfUsePage} />
      <Route path="/" component={Home} />
      <Route component={Home} />
    </Switch>
  );
}
