/**
 * Agent Tunnel Client Application
 * Handles authentication, WebSocket connection, and xterm.js terminal
 */

(function() {
    'use strict';

    // DOM Elements
    const loginOverlay = document.getElementById('login-overlay');
    const loginForm = document.getElementById('login-form');
    const usernameInput = document.getElementById('username');
    const passwordInput = document.getElementById('password');
    const errorMessage = document.getElementById('error-message');
    const remainingAttempts = document.getElementById('remaining-attempts');
    const terminalContainer = document.getElementById('terminal-container');
    const terminalDiv = document.getElementById('terminal');

    // State
    let terminal = null;
    let fitAddon = null;
    let ws = null;
    let isConnected = false;

    /**
     * Initialize the application
     */
    function init() {
        // Focus username field on load
        usernameInput.focus();

        // Handle form submission
        loginForm.addEventListener('submit', handleLogin);

        // Handle window resize
        window.addEventListener('resize', debounce(handleResize, 250));

        // Handle visibility change (pause/resume terminal)
        document.addEventListener('visibilitychange', handleVisibilityChange);
    }

    /**
     * Handle login form submission
     */
    async function handleLogin(e) {
        e.preventDefault();

        const username = usernameInput.value.trim();
        const password = passwordInput.value;

        if (!username || !password) {
            showError('Please enter both username and password');
            return;
        }

        setLoading(true);
        clearError();

        try {
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ username, password })
            });

            const data = await response.json();

            if (response.ok) {
                // Login successful
                showTerminal();
            } else {
                // Login failed
                showError(data.error || 'Authentication failed');
                if (data.remaining !== undefined) {
                    showRemainingAttempts(data.remaining);
                }
                passwordInput.value = '';
                passwordInput.focus();
            }
        } catch (error) {
            showError('Connection failed. Please try again.');
            console.error('Login error:', error);
        } finally {
            setLoading(false);
        }
    }

    /**
     * Show the terminal and initialize xterm.js
     */
    function showTerminal() {
        // Hide login overlay
        loginOverlay.classList.add('hidden');
        
        // Show terminal container
        terminalContainer.style.display = 'block';

        // Initialize terminal after a short delay to ensure container is visible
        setTimeout(() => {
            initializeTerminal();
            connectWebSocket();
        }, 300);
    }

    /**
     * Initialize xterm.js terminal
     */
    function initializeTerminal() {
        terminal = new Terminal({
            cursorBlink: true,
            fontSize: 14,
            fontFamily: 'Menlo, Monaco, "Courier New", monospace',
            theme: {
                background: '#1e1e1e',
                foreground: '#d4d4d4',
                cursor: '#d4d4d4',
                selectionBackground: '#264f78',
                black: '#1e1e1e',
                red: '#f48771',
                green: '#89d185',
                yellow: '#dcdcaa',
                blue: '#569cd6',
                magenta: '#c586c0',
                cyan: '#4ec9b0',
                white: '#d4d4d4'
            },
            scrollback: 10000,
            allowProposedApi: true
        });

        // Add fit addon
        fitAddon = new FitAddon.FitAddon();
        terminal.loadAddon(fitAddon);

        // Open terminal in container
        terminal.open(terminalDiv);

        // Fit terminal to container
        fitAddon.fit();

        // Handle input from terminal
        terminal.onData((data) => {
            if (ws && ws.readyState === WebSocket.OPEN) {
                ws.send(data);
            }
        });

        // Handle resize
        terminal.onResize(({ cols, rows }) => {
            if (ws && ws.readyState === WebSocket.OPEN) {
                const resizeMsg = JSON.stringify({
                    type: 'resize',
                    data: JSON.stringify({ cols, rows })
                });
                ws.send(resizeMsg);
            }
        });

        // Focus terminal
        terminal.focus();
    }

    /**
     * Connect to WebSocket server
     */
    function connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
            console.log('WebSocket connected');
            isConnected = true;
            
            // Send initial resize
            if (terminal && fitAddon) {
                const { cols, rows } = terminal;
                const resizeMsg = JSON.stringify({
                    type: 'resize',
                    data: JSON.stringify({ cols, rows })
                });
                ws.send(resizeMsg);
            }
        };

        ws.onmessage = (event) => {
            if (terminal) {
                // Handle binary data (terminal output)
                if (event.data instanceof Blob) {
                    const reader = new FileReader();
                    reader.onload = () => {
                        terminal.write(new Uint8Array(reader.result));
                    };
                    reader.readAsArrayBuffer(event.data);
                } else {
                    // Handle text data
                    terminal.write(event.data);
                }
            }
        };

        ws.onclose = (event) => {
            console.log('WebSocket closed:', event.code, event.reason);
            isConnected = false;
            
            if (terminal) {
                terminal.writeln('');
                terminal.writeln('\r\n\x1b[31mConnection closed.\x1b[0m');
                terminal.writeln('\x1b[33mRefresh the page to reconnect.\x1b[0m');
            }
        };

        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            showError('Connection error. Please refresh the page.');
        };
    }

    /**
     * Handle window resize
     */
    function handleResize() {
        if (fitAddon) {
            fitAddon.fit();
        }
    }

    /**
     * Handle visibility change (pause/resume)
     */
    function handleVisibilityChange() {
        if (document.hidden) {
            // Page is hidden - could pause updates if needed
            console.log('Page hidden');
        } else {
            // Page is visible again
            console.log('Page visible');
            if (terminal) {
                terminal.focus();
            }
        }
    }

    /**
     * Show error message
     */
    function showError(message) {
        errorMessage.textContent = message;
        remainingAttempts.textContent = '';
    }

    /**
     * Show remaining login attempts
     */
    function showRemainingAttempts(count) {
        if (count > 0) {
            remainingAttempts.textContent = `${count} attempt${count === 1 ? '' : 's'} remaining`;
        } else {
            remainingAttempts.textContent = 'Too many failed attempts. Please try again later.';
        }
    }

    /**
     * Clear error message
     */
    function clearError() {
        errorMessage.textContent = '';
        remainingAttempts.textContent = '';
    }

    /**
     * Set loading state
     */
    function setLoading(loading) {
        const btn = loginForm.querySelector('.login-btn');
        const btnText = btn.querySelector('.btn-text');
        const btnLoader = btn.querySelector('.btn-loader');

        btn.disabled = loading;
        btnText.style.display = loading ? 'none' : 'inline';
        btnLoader.style.display = loading ? 'inline' : 'none';
    }

    /**
     * Debounce function
     */
    function debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
