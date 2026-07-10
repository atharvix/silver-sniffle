import { useState } from 'react';
import LandingPage from './LandingPage';
import DiscoveryScreen from './DiscoveryScreen';

export default function App() {
  const [view, setView] = useState<'landing' | 'discovery'>('landing');

  if (view === 'discovery') {
    return <DiscoveryScreen onBack={() => setView('landing')} />;
  }

  return <LandingPage onDiscovery={() => setView('discovery')} />;
}
