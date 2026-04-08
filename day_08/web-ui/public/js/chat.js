// Chat UI module - pure functions for testing
export const createChatApp = (deps) => {
    const {
        agentsList,
        messagesContainer,
        messageInput,
        sendButton,
        status,
        errorBox,
        API_BASE = 'http://localhost:8080/api'
    } = deps;

    // Token display elements
    const tokenInfo = document.getElementById('tokenInfo');
    const currentTokens = document.getElementById('currentTokens');
    const tokenDetails = document.getElementById('tokenDetails');
    const promptTokens = document.getElementById('promptTokens');
    const completionTokens = document.getElementById('completionTokens');
    const totalTokens = document.getElementById('totalTokens');
    const historyTokens = document.getElementById('historyTokens');

    let currentAgent = null;
    let isLoading = false;
    let currentMessages = []; // Store messages in memory for consistent state

    // Update token estimate based on input text
    function updateTokenEstimate(text) {
        const approxTokens = Math.max(1, Math.floor(text.length / 4));
        if (currentTokens) currentTokens.textContent = approxTokens;
    }

    // Real-time token estimation
    if (messageInput) {
        messageInput.addEventListener('input', (e) => {
            updateTokenEstimate(e.target.value);
        });
    }

    // Fetch agents from API
    async function loadAgents() {
        try {
            status.textContent = 'Loading agents...';
            const response = await fetch(`${API_BASE}/agents`);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const agents = await response.json();
            renderAgents(agents);
            status.textContent = 'Select an agent to start chatting';
        } catch (error) {
            showError(`Failed to load agents: ${error.message}`);
        }
    }

    // Render agents list
    function renderAgents(agents) {
        agentsList.innerHTML = '';
        agents.forEach(agent => {
            const li = document.createElement('li');
            li.className = 'agent-item';
            li.innerHTML = `
                <div class="agent-name">${escapeHtml(agent.Name)}</div>
                <div class="agent-desc">${escapeHtml(agent.Description || 'No description')}</div>
            `;
            li.addEventListener('click', (e) => selectAgent(agent, e));
            agentsList.appendChild(li);
        });
    }

    // Select an agent
    async function selectAgent(agent, event) {
        currentAgent = agent;
        currentMessages = []; // Reset messages for new agent
        // Reset token display
        if (currentTokens) currentTokens.textContent = '0';
        if (promptTokens) promptTokens.textContent = '0';
        if (completionTokens) completionTokens.textContent = '0';
        if (totalTokens) totalTokens.textContent = '0';
        if (historyTokens) historyTokens.textContent = '0';
        if (tokenDetails) tokenDetails.classList.add('hidden');
        // Update UI
        document.querySelectorAll('.agent-item').forEach(item => item.classList.remove('active'));
        event.target.closest('.agent-item').classList.add('active');
        
        status.textContent = `Loading conversation with ${agent.Name}...`;
        messageInput.disabled = false;
        sendButton.disabled = false;
        messageInput.focus();
        
        // Load messages for this agent
        try {
            const response = await fetch(`${API_BASE}/agents/${encodeURIComponent(agent.Name)}/messages`);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const messages = await response.json();
            renderMessages(messages);
            status.textContent = `Ready to chat with ${agent.Name}`;
        } catch (error) {
            showError(`Failed to load messages: ${error.message}`);
            renderMessages([]);
        }
    }

    // Render messages - updates currentMessages state and renders to DOM
    function renderMessages(messages) {
        // Filter out system messages (they're in the prompt)
        currentMessages = messages.filter(msg => msg.role !== 'system');
        
        messagesContainer.innerHTML = '';
        
        if (currentMessages.length === 0) {
            messagesContainer.innerHTML = '<div class="message assistant">No messages yet. Start the conversation!</div>';
            return;
        }
        
        currentMessages.forEach(msg => {
            const div = document.createElement('div');
            div.className = `message ${msg.role}`;
            div.textContent = msg.content;
            messagesContainer.appendChild(div);
        });
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
    }

    // Send message
    async function sendMessage() {
        if (!currentAgent || isLoading) return;
        
        const text = messageInput.value.trim();
        if (!text) return;
        
        // Add user message to UI immediately
        const userMsg = { role: 'user', content: text };
        renderMessages([...currentMessages, userMsg]);
        messageInput.value = '';
        updateTokenEstimate('');
        messageInput.disabled = true;
        sendButton.disabled = true;
        isLoading = true;
        status.textContent = 'Thinking...';
        
        try {
            const response = await fetch(`${API_BASE}/agents/${encodeURIComponent(currentAgent.Name)}/messages`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ message: text })
            });
            
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            
            // Add assistant response
            const assistantMsg = { role: 'assistant', content: data.response };
            renderMessages([...currentMessages, assistantMsg]);
            status.textContent = `Ready to chat with ${currentAgent.Name}`;
            
            // Update token counts if provided
            if (data.token_counts) {
                if (promptTokens) promptTokens.textContent = data.token_counts.prompt_tokens;
                if (completionTokens) completionTokens.textContent = data.token_counts.completion_tokens;
                if (totalTokens) totalTokens.textContent = data.token_counts.total_tokens;
                if (historyTokens) historyTokens.textContent = data.token_counts.history_tokens;
                if (tokenDetails) tokenDetails.classList.remove('hidden');
            }
        } catch (error) {
            showError(`Failed to send message: ${error.message}`);
            status.textContent = 'Error - try again';
        } finally {
            messageInput.disabled = false;
            sendButton.disabled = false;
            isLoading = false;
            messageInput.focus();
        }
    }

    // Show error
    function showError(message) {
        errorBox.textContent = message;
        errorBox.classList.remove('hidden');
        setTimeout(() => errorBox.classList.add('hidden'), 5000);
    }

    // Escape HTML to prevent XSS
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Public API
    return {
        loadAgents,
        renderAgents,
        selectAgent,
        renderMessages,
        sendMessage,
        showError,
        escapeHtml,
        // Expose state for testing
        getCurrentAgent: () => currentAgent,
        getCurrentMessages: () => currentMessages,
        getIsLoading: () => isLoading
    };
};