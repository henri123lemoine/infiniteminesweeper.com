import React from 'react';
import { usePWAInstall } from './hooks/usePWAInstall.js';

/**
 * PWA Install Banner Component
 * Shows an install prompt when the app can be installed
 */
function PWAInstallBanner() {
  const { canInstall, isInstalled, install, dismissInstall } = usePWAInstall();

  // Don't show banner if already installed or can't install
  if (!canInstall || isInstalled) {
    return null;
  }

  const handleInstall = async () => {
    const success = await install();
    if (!success) {
      // Could show an error message here
      console.log('Installation failed or was cancelled');
    }
  };

  return (
    <div
      style={{
        position: 'fixed',
        bottom: 20,
        left: 20,
        right: 20,
        background: '#1976d2',
        color: 'white',
        padding: '12px 16px',
        borderRadius: 8,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        boxShadow: '0 4px 12px rgba(0,0,0,0.3)',
        zIndex: 1000,
        maxWidth: 600,
        margin: '0 auto',
      }}
    >
      <div style={{ flex: 1 }}>
        <div style={{ fontWeight: 'bold', marginBottom: 4 }}>
          Install Infinite Minesweeper
        </div>
        <div style={{ fontSize: 14, opacity: 0.9 }}>
          Add to your home screen for the best experience
        </div>
      </div>
      
      <div style={{ display: 'flex', gap: 8, marginLeft: 16 }}>
        <button
          onClick={dismissInstall}
          style={{
            background: 'transparent',
            border: '1px solid rgba(255,255,255,0.3)',
            color: 'white',
            padding: '6px 12px',
            borderRadius: 4,
            cursor: 'pointer',
            fontSize: 14,
          }}
        >
          Maybe Later
        </button>
        <button
          onClick={handleInstall}
          style={{
            background: 'white',
            border: 'none',
            color: '#1976d2',
            padding: '6px 16px',
            borderRadius: 4,
            cursor: 'pointer',
            fontWeight: 'bold',
            fontSize: 14,
          }}
        >
          Install
        </button>
      </div>
    </div>
  );
}

export default PWAInstallBanner;