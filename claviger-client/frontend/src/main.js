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

        if (status === "unregistered" || status === "pending_approval") {
            showScreen('view-enroll');
        } else if (status === "active") {
            document.getElementById('dash-ip').innerText = ip;
            document.getElementById('dash-endpoint').innerText = endpoint;
            showScreen('view-dashboard');
            
            // Sync the toggle button with the actual engine state
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
        
        // Show the copy button container
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
        
        // Temporary success state for UX
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
// DASHBOARD ACTIONS
// ==========================================
async function syncToggleUI() {
    const isConnected = await window.go.main.App.IsConnected();
    const btnText = document.getElementById('toggle-text');
    const indicator = document.getElementById('status-indicator');

    if (isConnected) {
        btnText.innerText = "Disconnect Tunnel";
        indicator.classList.replace('bg-slate-500', 'bg-success');
        indicator.classList.add('shadow-[0_0_8px_rgba(34,197,94,0.8)]');
    } else {
        btnText.innerText = "Connect Tunnel";
        indicator.classList.replace('bg-success', 'bg-slate-500');
        indicator.classList.remove('shadow-[0_0_8px_rgba(34,197,94,0.8)]');
    }
}

async function handleToggleVPN() {
    const btnText = document.getElementById('toggle-text');
    
    try {
        const isConnected = await window.go.main.App.IsConnected();
        
        if (isConnected) {
            btnText.innerText = "Disconnecting...";
            await window.go.main.App.Disconnect();
        } else {
            btnText.innerText = "Connecting...";
            await window.go.main.App.Connect();
        }
        
        await syncToggleUI();
    } catch (err) {
        alert("VPN Engine Error: " + err);
        await syncToggleUI();
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