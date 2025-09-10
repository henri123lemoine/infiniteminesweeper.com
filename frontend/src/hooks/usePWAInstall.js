import { useState, useEffect, useCallback } from 'react';

/**
 * Hook to manage PWA installation
 * Handles beforeinstallprompt event and provides installation controls
 */
export function usePWAInstall() {
  const [canInstall, setCanInstall] = useState(false);
  const [installPrompt, setInstallPrompt] = useState(null);
  const [isInstalled, setIsInstalled] = useState(false);

  useEffect(() => {
    // Check if app is already installed
    const checkInstalled = () => {
      const isStandalone = window.matchMedia('(display-mode: standalone)').matches;
      const isInWebAppiOS = window.navigator.standalone === true;
      const isInWebAppChrome = window.matchMedia('(display-mode: standalone)').matches;
      
      setIsInstalled(isStandalone || isInWebAppiOS || isInWebAppChrome);
    };

    checkInstalled();

    // Listen for beforeinstallprompt event
    const handleBeforeInstallPrompt = (e) => {
      console.log('[PWA] beforeinstallprompt event fired');
      // Prevent the default prompt
      e.preventDefault();
      // Save the event for later use
      setInstallPrompt(e);
      setCanInstall(true);
    };

    // Listen for app installation
    const handleAppInstalled = () => {
      console.log('[PWA] App was installed');
      setIsInstalled(true);
      setCanInstall(false);
      setInstallPrompt(null);
    };

    // Register service worker
    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.register('/sw.js')
        .then((registration) => {
          console.log('[PWA] Service Worker registered successfully:', registration.scope);
        })
        .catch((error) => {
          console.log('[PWA] Service Worker registration failed:', error);
        });
    }

    window.addEventListener('beforeinstallprompt', handleBeforeInstallPrompt);
    window.addEventListener('appinstalled', handleAppInstalled);

    return () => {
      window.removeEventListener('beforeinstallprompt', handleBeforeInstallPrompt);
      window.removeEventListener('appinstalled', handleAppInstalled);
    };
  }, []);

  // Function to trigger installation
  const install = useCallback(async () => {
    if (!installPrompt) {
      return false;
    }

    try {
      // Show the installation prompt
      const result = await installPrompt.prompt();
      
      // Wait for the user to respond to the prompt
      const choiceResult = await result;
      
      if (choiceResult.outcome === 'accepted') {
        console.log('[PWA] User accepted the install prompt');
        setCanInstall(false);
        setInstallPrompt(null);
        return true;
      } else {
        console.log('[PWA] User dismissed the install prompt');
        return false;
      }
    } catch (error) {
      console.error('[PWA] Error during installation:', error);
      return false;
    }
  }, [installPrompt]);

  // Function to dismiss the install prompt
  const dismissInstall = useCallback(() => {
    setCanInstall(false);
    setInstallPrompt(null);
  }, []);

  return {
    canInstall,
    isInstalled,
    install,
    dismissInstall
  };
}