import { useState } from 'react';
import Welcome from './components/Welcome.jsx';
import Main from './components/Main.jsx';

export default function App() {
  const [displayName, setDisplayName] = useState(() => localStorage.getItem('displayName'));

  if (!displayName) {
    return <Welcome onRegistered={setDisplayName} />;
  }

  return (
    <Main
      displayName={displayName}
      onLogout={() => {
        localStorage.removeItem('displayName');
        setDisplayName(null);
      }}
      onRename={setDisplayName}
    />
  );
}