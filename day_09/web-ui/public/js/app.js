import { createChatApp } from './chat.js';

// DOM elements
const agentsList = document.getElementById('agentsList');
const messagesContainer = document.getElementById('messagesContainer');
const messageInput = document.getElementById('messageInput');
const sendButton = document.getElementById('sendButton');
const status = document.getElementById('status');
const errorBox = document.getElementById('errorBox');

const API_BASE = 'http://localhost:8080/api';

// Create chat app instance
const chatApp = createChatApp({
    agentsList,
    messagesContainer,
    messageInput,
    sendButton,
    status,
    errorBox,
    API_BASE
});

// Event listeners
sendButton.addEventListener('click', () => chatApp.sendMessage());
messageInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        chatApp.sendMessage();
    }
});

// Initialize
chatApp.loadAgents();