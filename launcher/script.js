/**
 * Maxx Launcher Script
 * Single page app with launcher and settings views
 */
(function() {
    'use strict';

    // Config
    const CONFIG = {
        checkInterval: 500,
        maxWaitTime: 60000,
        redirectDelay: 300
    };

    // Pages
    const pages = {
        launcher: document.getElementById('page-launcher'),
        settings: document.getElementById('page-settings')
    };

    // Launcher elements
    const launcher = {
        statusContainer: document.getElementById('status-container'),
        statusText: document.getElementById('status-text'),
        loadingSpinner: document.getElementById('loading-spinner'),
        errorContainer: document.getElementById('error-container'),
        errorMessage: document.getElementById('error-message'),
        retryButton: document.getElementById('retry-button'),
        quitButton: document.getElementById('quit-button'),
        versionText: document.getElementById('version-text')
    };

    // Settings elements
    const settings = {
        portInput: document.getElementById('port-input'),
        datadirInput: document.getElementById('datadir-input'),
        saveButton: document.getElementById('save-button'),
        backButton: document.getElementById('back-button')
    };

    // State
    let checkTimer = null;
    let startTime = Date.now();

    // ==================== Page Navigation ====================

    function showPage(name) {
        Object.keys(pages).forEach(key => {
            pages[key].classList.toggle('active', key === name);
        });
        if (name === 'settings') {
            loadSettings();
        }
    }

    function getPageFromUrl() {
        const params = new URLSearchParams(window.location.search);
        return params.get('page') || 'launcher';
    }

    // Expose for menu
    window.showSettingsPage = function() {
        showPage('settings');
        history.replaceState(null, '', '?page=settings');
    };

    window.showLauncherPage = function() {
        showPage('launcher');
        history.replaceState(null, '', '?page=launcher');
    };

    // ==================== Toast ====================

    function showToast(message, type = 'success') {
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.textContent = message;
        document.body.appendChild(toast);
        setTimeout(() => toast.remove(), 3000);
    }

    // ==================== Launcher Functions ====================

    function updateStatus(text) {
        launcher.statusText.textContent = text;
    }

    function showError(message) {
        launcher.statusContainer.style.display = 'none';
        launcher.errorContainer.classList.remove('hidden');
        launcher.errorMessage.textContent = message;
    }

    function hideError() {
        launcher.errorContainer.classList.add('hidden');
        launcher.statusContainer.style.display = 'flex';
        launcher.statusContainer.classList.remove('success');
    }

    function showSuccess(message) {
        launcher.statusContainer.classList.add('success');
        updateStatus(message || 'Ready, redirecting...');
    }

    function redirectTo(url) {
        showSuccess('Ready, redirecting...');
        setTimeout(() => {
            window.location.href = url;
        }, CONFIG.redirectDelay);
    }

    async function checkServer() {
        const elapsed = Date.now() - startTime;

        if (elapsed > CONFIG.maxWaitTime) {
            clearInterval(checkTimer);
            showError('Server startup timeout\n\nPlease check the log files or retry.');
            return;
        }

        const seconds = Math.floor(elapsed / 1000);
        if (seconds > 0) {
            updateStatus(`Starting service... (${seconds}s)`);
        }

        try {
            if (!window.go || !window.go.desktop || !window.go.desktop.LauncherApp) {
                console.log('[Launcher] Waiting for Wails runtime...');
                return;
            }

            const status = await window.go.desktop.LauncherApp.CheckServerStatus();

            if (status.Message) {
                updateStatus(status.Message);
            }

            if (status.Ready && status.RedirectURL) {
                clearInterval(checkTimer);
                redirectTo(status.RedirectURL);
                return;
            }

            if (status.Error) {
                clearInterval(checkTimer);
                showError(status.Error);
                return;
            }
        } catch (err) {
            console.error('[Launcher] Check status failed:', err);
        }
    }

    async function retry() {
        hideError();
        startTime = Date.now();
        updateStatus('Restarting service...');

        try {
            if (window.go && window.go.desktop && window.go.desktop.LauncherApp) {
                await window.go.desktop.LauncherApp.RestartServer();
            }
        } catch (err) {
            console.error('[Launcher] Restart failed:', err);
            showError('Failed to restart server: ' + (err.message || err));
            return;
        }

        checkTimer = setInterval(checkServer, CONFIG.checkInterval);
        checkServer();
    }

    function quit() {
        if (window.go && window.go.desktop && window.go.desktop.LauncherApp) {
            window.go.desktop.LauncherApp.Quit();
        } else {
            window.close();
        }
    }

    async function loadVersion() {
        const maxWait = 5000;
        const startWait = Date.now();

        while (Date.now() - startWait < maxWait) {
            if (window.go && window.go.desktop && window.go.desktop.LauncherApp) {
                try {
                    const version = await window.go.desktop.LauncherApp.GetVersion();
                    if (version) {
                        launcher.versionText.textContent = version;
                    }
                } catch (err) {
                    console.error('[Launcher] Failed to get version:', err);
                }
                return;
            }
            await new Promise(resolve => setTimeout(resolve, 100));
        }
    }

    // ==================== Settings Functions ====================

    async function loadSettings() {
        console.log('[Settings] Loading settings...');

        if (!window.go || !window.go.desktop || !window.go.desktop.LauncherApp) {
            showToast('Wails runtime not ready', 'error');
            return;
        }

        try {
            const config = await window.go.desktop.LauncherApp.GetConfig();
            const dataDir = await window.go.desktop.LauncherApp.GetDataDir();

            settings.portInput.value = config.port || 9880;
            settings.datadirInput.value = dataDir || '';

            console.log('[Settings] Config loaded:', config, 'dataDir:', dataDir);
        } catch (err) {
            console.error('[Settings] Failed to load config:', err);
            showToast('Failed to load config: ' + (err.message || err), 'error');
        }
    }

    async function saveSettings() {
        const port = parseInt(settings.portInput.value, 10);

        if (isNaN(port) || port < 1 || port > 65535) {
            showToast('Port must be between 1-65535', 'error');
            return;
        }

        try {
            if (!window.go || !window.go.desktop || !window.go.desktop.LauncherApp) {
                showToast('Wails runtime not ready', 'error');
                return;
            }

            settings.saveButton.disabled = true;
            settings.saveButton.textContent = 'Saving...';

            await window.go.desktop.LauncherApp.SaveConfig({ port: port });

            showToast('Config saved, restarting service...');

            await window.go.desktop.LauncherApp.RestartServer();

            // Go back to launcher and restart checking
            showPage('launcher');
            history.replaceState(null, '', '?page=launcher');
            startTime = Date.now();
            hideError();
            checkTimer = setInterval(checkServer, CONFIG.checkInterval);
            checkServer();

        } catch (err) {
            console.error('[Settings] Failed to save config:', err);
            showToast('Failed to save config: ' + (err.message || err), 'error');
        } finally {
            settings.saveButton.disabled = false;
            settings.saveButton.textContent = 'Save & Restart';
        }
    }

    function goBack() {
        window.location.href = 'wails://wails/index.html';
    }

    // ==================== Initialization ====================

    function init() {
        console.log('[App] Initializing...');

        // Launcher events
        launcher.retryButton.addEventListener('click', retry);
        launcher.quitButton.addEventListener('click', quit);

        // Settings events
        settings.saveButton.addEventListener('click', saveSettings);
        settings.backButton.addEventListener('click', goBack);

        // Load version
        loadVersion();

        // Check URL for page
        const page = getPageFromUrl();
        if (page === 'settings') {
            showPage('settings');
        } else {
            // Start checking server status
            checkTimer = setInterval(checkServer, CONFIG.checkInterval);
            checkServer();
        }
    }

    // Wait for DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
