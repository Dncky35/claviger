// ==========================================
// GLOBAL STATE & ERROR HANDLING
// ==========================================
let currentVault = null;
let generatedRequestToken = "";

// Catch any fatal JavaScript errors and print them to the screen
window.onerror = function(msg, url, line) {
    const errBox = document.getElementById('fatal-error-log');
    errBox.innerText = `FATAL JS ERROR: ${msg} (Line ${line})`;
    errBox.classList.remove('hidden');
};

// Wait for Wails bindings to inject
window.onload = () => {
    setTimeout(async () => {
        // 1. Hook into Wails Native Events to listen for the Background Watchdog!
        if (window.runtime && window.runtime.EventsOn) {
            window.runtime.EventsOn("vpn-state-change", (newState) => {
                console.log("📡 Engine State Push Received:", newState);
                syncToggleUI(newState);
            });
        }
        
        await refreshState();
    }, 100);
};

// ==========================================
// STATE MANAGEMENT & NAVIGATION
// ==========================================
async function refreshState() {
    try {
        // Ensure Go bindings exist
        if (!window.go || !window.go.main || !window.go.main.App) {
            throw new Error("Wails Go bindings failed to load. Check app.go bindings in main.go.");
        }

        currentVault = await window.go.main.App.GetVault();
        
        // Check properties carefully
        const status = currentVault.Status || currentVault.status || "unregistered";
        const ip = currentVault.AssignedIP || currentVault.assigned_ip || "Unknown";
        const endpoint = currentVault.ServerEndpoint || currentVault.server_endpoint || "Connected";
        
        // Load the Global Routing preference from the vault
        const useGlobalRouting = currentVault.UseGlobalRouting || currentVault.use_global_routing || false;

        if (status === "unregistered" || status === "pending_approval") {
            showScreen('view-enroll');
        } else if (status === "active") {
            document.getElementById('dash-ip').innerText = ip;
            document.getElementById('dash-endpoint').innerText = endpoint;
            
            // Set the toggle switch state
            document.getElementById('toggle-global-route').checked = useGlobalRouting;
            
            showScreen('view-dashboard');
            
            // Sync the UI with the actual 5-state engine
            await syncToggleUI();
        } else {
            showScreen('view-enroll');
        }
    } catch (err) {
        const errBox = document.getElementById('fatal-error-log');
        errBox.innerText = `GO BINDING ERROR: ${err}`;
        errBox.classList.remove('hidden');
        showScreen('view-enroll'); // Fail open so the user sees the error
    }
}

function showScreen(screenId) {
    document.getElementById('view-enroll').classList.add('hidden');
    document.getElementById('view-dashboard').classList.add('hidden');
    
    const target = document.getElementById(screenId);
    target.classList.remove('hidden');
    target.classList.add('animate-fade-in');
}

// ==========================================
// ENROLLMENT ACTIONS
// ==========================================
async function handleGenerateRequest() {
    const btn = document.getElementById('btn-generate-req');
    btn.innerText = "Generating...";
    btn.disabled = true;

    try {
        generatedRequestToken = await window.go.main.App.GenerateRequest();
        
        document.getElementById('req-copy-container').classList.remove('hidden');
        
        btn.innerText = "Regenerate Token";
        btn.disabled = false;
    } catch (err) {
        alert("Error generating request: " + err);
        btn.innerText = "Generate Request Token";
        btn.disabled = false;
    }
}

function copyClientRequest() {
    if (!generatedRequestToken) return;

    navigator.clipboard.writeText(generatedRequestToken).then(() => {
        const copyBtn = document.getElementById('btn-copy-req');
        const originalHTML = copyBtn.innerHTML;
        
        copyBtn.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg> Copied!`;
        copyBtn.classList.replace('text-primary', 'text-success');
        copyBtn.classList.replace('bg-primary/20', 'bg-success/20');
        copyBtn.classList.replace('border-primary/30', 'border-success/30');

        setTimeout(() => { 
            copyBtn.innerHTML = originalHTML; 
            copyBtn.classList.replace('text-success', 'text-primary');
            copyBtn.classList.replace('bg-success/20', 'bg-primary/20');
            copyBtn.classList.replace('border-success/30', 'border-primary/30');
        }, 2000);
    }).catch(err => console.error("Clipboard failed", err));
}

async function handleVerify() {
    const tokenInput = document.getElementById('input-visa').value.trim();
    const btn = document.getElementById('btn-verify');
    const errText = document.getElementById('enroll-error');

    if (!tokenInput) {
        errText.innerText = "Please paste the Server Approval Token.";
        errText.classList.remove('hidden');
        return;
    }

    btn.innerText = "Verifying...";
    btn.disabled = true;
    errText.classList.add('hidden');

    try {
        await window.go.main.App.ProcessApproval(tokenInput);
        document.getElementById('input-visa').value = '';
        btn.innerText = "Verify";
        btn.disabled = false;
        await refreshState();
    } catch (err) {
        errText.innerText = err;
        errText.classList.remove('hidden');
        btn.innerText = "Verify";
        btn.disabled = false;
    }
}

// ==========================================
// DASHBOARD ACTIONS & DYNAMIC STATE UI
// ==========================================
async function syncToggleUI(forcedState = null) {
    const state = forcedState || await window.go.main.App.GetTunnelState();
    
    // Bottom Button Elements
    const btnText = document.getElementById('toggle-text');
    const btnIndicator = document.getElementById('btn-status-indicator');
    
    // Shield Elements
    const shieldTitle = document.getElementById('status-title');
    const shieldSubtitle = document.getElementById('status-subtitle');
    const shieldBg = document.getElementById('status-shield');
    const shieldIcon = document.getElementById('status-icon');
    const ring1 = document.getElementById('status-ring-1');
    const ring2 = document.getElementById('status-ring-2');

    // Routing Toggle Elements
    const routeToggle = document.getElementById('toggle-global-route');
    const routeWarning = document.getElementById('route-warning');

    // 1. Wipe previous colors/animations
    btnIndicator.className = "w-3 h-3 rounded-full transition-all duration-300";
    shieldBg.className = "relative z-10 w-20 h-20 rounded-2xl flex items-center justify-center shadow-lg transition-colors duration-300";
    ring1.classList.add('hidden');
    ring2.classList.add('hidden');

    // 2. Lock or Unlock the Route Switch
    if (state === "disconnected") {
        routeToggle.disabled = false;
        routeWarning.classList.add('hidden');
    } else {
        routeToggle.disabled = true;
        routeWarning.classList.remove('hidden');
    }

    // 3. Apply Visual States
    switch (state) {
        case "disconnected":
            btnText.innerText = "Connect Tunnel";
            btnIndicator.classList.add('bg-slate-500');
            
            shieldTitle.innerText = "Disconnected";
            shieldSubtitle.innerText = "Tunnel Inactive";
            shieldBg.classList.add('bg-slate-700');
            shieldIcon.innerHTML = `<rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path>`;
            shieldIcon.classList.replace('text-white', 'text-slate-400');
            break;

        case "connecting":
            btnText.innerText = "Building Tunnel...";
            btnIndicator.classList.add('bg-yellow-400', 'animate-pulse', 'shadow-[0_0_8px_rgba(250,204,21,0.8)]');
            
            shieldTitle.innerText = "Connecting";
            shieldSubtitle.innerText = "Building Interface";
            shieldBg.classList.add('bg-yellow-500');
            shieldIcon.innerHTML = `<path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"></path><polyline points="21 3 21 8 16 8"></polyline>`;
            shieldIcon.classList.replace('text-slate-400', 'text-white');
            break;

        case "verifying":
            btnText.innerText = "Verifying Handshake...";
            btnIndicator.classList.add('bg-blue-400', 'animate-pulse', 'shadow-[0_0_8px_rgba(96,165,250,0.8)]');
            
            shieldTitle.innerText = "Verifying";
            shieldSubtitle.innerText = "Awaiting Handshake";
            shieldBg.classList.add('bg-blue-500');
            shieldIcon.innerHTML = `<rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path>`;
            shieldIcon.classList.replace('text-slate-400', 'text-white');
            break;

        case "secured":
            btnText.innerText = "Disconnect Tunnel";
            btnIndicator.classList.add('bg-success', 'shadow-[0_0_8px_rgba(34,197,94,0.8)]');
            
            shieldTitle.innerText = "Secured";
            shieldSubtitle.innerText = "Zero-Trust Tunnel Active";
            shieldBg.classList.add('bg-success', 'shadow-[0_0_30px_rgba(34,197,94,0.4)]');
            shieldIcon.innerHTML = `<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><polyline points="9 12 11 14 15 10"/>`;
            shieldIcon.classList.replace('text-slate-400', 'text-success');
            
            ring1.classList.remove('hidden');
            ring2.classList.remove('hidden');
            ring1.className = "absolute inset-0 bg-success/20 rounded-full animate-ping hidden".replace('hidden', '');
            ring1.style.animationDuration = "3s";
            ring2.className = "absolute inset-2 bg-success/30 rounded-full animate-pulse hidden".replace('hidden', '');
            break;

        case "reconnecting":
            btnText.innerText = "Ghost Connection! Reconnecting...";
            btnIndicator.classList.add('bg-orange-500', 'animate-ping', 'shadow-[0_0_8px_rgba(249,115,22,0.8)]');
            
            shieldTitle.innerText = "Ghost Connection";
            shieldSubtitle.innerText = "Searching for Server";
            shieldBg.classList.add('bg-orange-500');
            shieldIcon.innerHTML = `<path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line>`;
            shieldIcon.classList.replace('text-slate-400', 'text-white');
            break;

        default:
            btnText.innerText = "Unknown State";
            btnIndicator.classList.add('bg-destructive');
            break;
    }
}

async function handleToggleVPN() {
    try {
        const currentState = await window.go.main.App.GetTunnelState();
        
        if (currentState === "disconnected") {
            await window.go.main.App.Connect();
        } else {
            await window.go.main.App.Disconnect();
        }
    } catch (err) {
        alert("VPN Engine Error: " + err);
        await syncToggleUI(); // Fallback sync
    }
}

// NEW: Handle the Global Routing Switch Click
async function handleToggleRouting() {
    const toggle = document.getElementById('toggle-global-route');
    const isEnabled = toggle.checked;

    try {
        // Call the Go backend to save this preference to the vault
        await window.go.main.App.ToggleGlobalRouting(isEnabled);
        console.log("Global routing updated in vault:", isEnabled);
    } catch (err) {
        alert("Failed to save routing preference: " + err);
        // If the Go backend fails, revert the switch UI
        toggle.checked = !isEnabled;
    }
}

async function handleLeaveNetwork() {
    if (!confirm("🚨 DANGER: Are you sure you want to forget this network? You will need to ask the Administrator for a new Visa to rejoin.")) return;

    try {
        await window.go.main.App.LeaveNetwork();
        await refreshState(); 
    } catch (err) {
        alert("Error leaving network: " + err);
    }
}