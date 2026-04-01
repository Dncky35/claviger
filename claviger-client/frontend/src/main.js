// State variables
let currentVault = null;
let isConnected = false;
let pollingInterval = null;

// Wait for Wails to fully inject its Go bindings before starting
window.onload = async () => {
    // Ask Go for the current vault data on startup
    await refreshState();
};

// Fetches the Vault from Go and decides which screen to show
async function refreshState() {
    try {
        currentVault = await window.go.main.App.GetVault();
        
        if (currentVault.status === "unregistered" || !currentVault.status) {
            showScreen('screen-enroll');
        } else if (currentVault.status === "pending") {
            showScreen('screen-waiting');
            startPolling(); // Start asking the server if we are approved yet
        } else if (currentVault.status === "active") {
            document.getElementById('dash-ip').innerText = currentVault.assigned_ip;
            
            // Check if the Go Engine says the VPN is currently running
            isConnected = await window.go.main.App.IsConnected();
            updateDashboardUI();
            
            showScreen('screen-dashboard');
            startPolling(); // Keep slow-polling to see if the Admin revoked us
        }
    } catch (err) {
        console.error("Failed to load state from Go:", err);
    }
}

// Switches between the 3 UI screens
function showScreen(screenId) {
    document.getElementById('screen-enroll').classList.add('hidden');
    document.getElementById('screen-waiting').classList.add('hidden');
    document.getElementById('screen-dashboard').classList.add('hidden');
    
    document.getElementById(screenId).classList.remove('hidden');
}

// ==========================================
// ACTION: ENROLLMENT (SMART TOKEN LOGIC)
// ==========================================
async function doEnroll() {
    const tokenInput = document.getElementById('input-token').value.trim();
    const btn = document.getElementById('btn-enroll');
    const errText = document.getElementById('enroll-error');

    if (!tokenInput) {
        errText.innerText = "Please paste your Smart Token.";
        errText.classList.remove('hidden');
        return;
    }

    btn.innerText = "Decrypting & Connecting...";
    btn.disabled = true;
    errText.classList.add('hidden');

    let serverUrl = "";
    let rawToken = "";

    try {
        // 1. Decode the Base64 Smart Token
        const decoded = atob(tokenInput);
        const parsed = JSON.parse(decoded);
        
        // Ensure all pieces are there
        if (!parsed.token || !parsed.server_ip || !parsed.hub_port) {
            throw new Error("Invalid token format.");
        }

        // 2. Reconstruct the Server URL and extract the raw token
        serverUrl = `http://${parsed.server_ip}:${parsed.hub_port}`;
        rawToken = parsed.token;

    } catch (err) {
        errText.innerText = "Invalid token. Please make sure you copied the exact string provided by your admin.";
        errText.classList.remove('hidden');
        btn.innerText = "Connect to Hub";
        btn.disabled = false;
        return; // Stop execution if token is bad
    }

    try {
        // 3. CALL THE GO FUNCTION with the unpacked data!
        await window.go.main.App.Enroll(serverUrl, rawToken);
        
        // If successful, reload the state (moves us to waiting room)
        await refreshState();
    } catch (err) {
        errText.innerText = err;
        errText.classList.remove('hidden');
        btn.innerText = "Connect to Hub";
        btn.disabled = false;
    }
}

// ==========================================
// ACTION: WAITING ROOM & REVOCATION POLLING
// ==========================================
function startPolling() {
    if (pollingInterval) clearInterval(pollingInterval);
    
    // Fast polling (3s) if waiting, Slow polling (15s) if active to check for revocation
    const intervalTime = (currentVault && currentVault.status === "active") ? 15000 : 3000;
    
    pollingInterval = setInterval(async () => {
        try {
            const newStatus = await window.go.main.App.CheckApproval();
            
            // If our status changed (e.g. pending -> active, or active -> unregistered)
            if (newStatus !== currentVault.status) {
                await refreshState(); 
            }
        } catch (err) {
            console.log("Polling error:", err);
        }
    }, intervalTime);
}

function stopPolling() {
    if (pollingInterval) {
        clearInterval(pollingInterval);
        pollingInterval = null;
    }
}

// ==========================================
// ACTION: VPN DASHBOARD TOGGLE
// ==========================================
async function toggleVPN() {
    const btn = document.getElementById('btn-toggle');
    btn.classList.add('scale-95', 'opacity-80'); // Click animation

    try {
        if (isConnected) {
            // CALL GO TO DISCONNECT
            await window.go.main.App.Disconnect();
            isConnected = false;
        } else {
            // CALL GO TO CONNECT
            await window.go.main.App.Connect();
            isConnected = true;
        }
    } catch (err) {
        alert("VPN Error: " + err);
    }

    // Restore animation and update colors
    setTimeout(() => btn.classList.remove('scale-95', 'opacity-80'), 150);
    updateDashboardUI();
}

// Updates the colors of the giant toggle button
function updateDashboardUI() {
    const statusText = document.getElementById('status-text');
    const powerIcon = document.getElementById('icon-power');
    const glow = document.getElementById('glow-active');
    const btn = document.getElementById('btn-toggle');

    if (isConnected) {
        statusText.innerText = "Connected";
        statusText.classList.replace('text-slate-400', 'text-emerald-400');
        powerIcon.classList.replace('text-slate-500', 'text-emerald-400');
        btn.classList.replace('border-slate-700', 'border-emerald-500/50');
        glow.classList.replace('shadow-[0_0_40px_rgba(34,197,94,0)]', 'shadow-[0_0_40px_rgba(34,197,94,0.3)]');
    } else {
        statusText.innerText = "Disconnected";
        statusText.classList.replace('text-emerald-400', 'text-slate-400');
        powerIcon.classList.replace('text-emerald-400', 'text-slate-500');
        btn.classList.replace('border-emerald-500/50', 'border-slate-700');
        glow.classList.replace('shadow-[0_0_40px_rgba(34,197,94,0.3)]', 'shadow-[0_0_40px_rgba(34,197,94,0)]');
    }
}

// ==========================================
// ACTION: LEAVE NETWORK
// ==========================================
async function leaveNetwork() {
    if (!confirm("Are you sure you want to disconnect and forget this network? You will need a new invite token to rejoin.")) {
        return;
    }

    try {
        await window.go.main.App.LeaveNetwork();
        stopPolling();
        isConnected = false;
        await refreshState(); // This will automatically throw them back to the Enroll screen!
    } catch (err) {
        alert("Error leaving network: " + err);
    }
}