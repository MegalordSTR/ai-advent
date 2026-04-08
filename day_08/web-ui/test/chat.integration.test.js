import { createChatApp } from '../public/js/chat.js';

// Mock fetch globally
global.fetch = jest.fn();

describe('Chat UI Integration', () => {
    let chatApp;
    let agentsList;
    let messagesContainer;
    let messageInput;
    let sendButton;
    let status;
    let errorBox;

    beforeEach(() => {
        // Clear all mocks
        fetch.mockClear();
        
        // Create real DOM elements
        agentsList = document.createElement('ul');
        agentsList.id = 'agentsList';
        
        messagesContainer = document.createElement('div');
        messagesContainer.id = 'messagesContainer';
        
        messageInput = document.createElement('textarea');
        messageInput.id = 'messageInput';
        
        sendButton = document.createElement('button');
        sendButton.id = 'sendButton';
        
        status = document.createElement('div');
        status.id = 'status';
        
        errorBox = document.createElement('div');
        errorBox.id = 'errorBox';
        errorBox.classList.add('hidden');
        
        // Create chat app with real elements
        chatApp = createChatApp({
            agentsList,
            messagesContainer,
            messageInput,
            sendButton,
            status,
            errorBox,
            API_BASE: 'http://localhost:8080/api'
        });
    });

    describe('Agent selection', () => {
        test('selectAgent loads and displays messages', async () => {
            const mockAgent = {
                Name: 'Помощник',
                Description: 'Test agent',
            };
            
            // Create a mock agent item
            const agentItem = document.createElement('li');
            agentItem.className = 'agent-item';
            agentsList.appendChild(agentItem);
            
            const mockMessages = [
                { Role: 'user', Content: 'Hello' },
                { Role: 'assistant', Content: 'Hi there' },
            ];
            
            // Mock successful fetch for messages
            fetch.mockResolvedValueOnce({
                ok: true,
                json: async () => mockMessages,
            });
            
            // Simulate click event
            const mockEvent = {
                target: agentItem,
            };
            
            await chatApp.selectAgent(mockAgent, mockEvent);
            
            // Verify current agent is set
            expect(chatApp.getCurrentAgent()).toEqual(mockAgent);
            
            // Verify fetch was called with encoded agent name
            expect(fetch).toHaveBeenCalledWith(
                'http://localhost:8080/api/agents/%D0%9F%D0%BE%D0%BC%D0%BE%D1%89%D0%BD%D0%B8%D0%BA/messages'
            );
            
            // Verify messages are rendered
            expect(chatApp.getCurrentMessages()).toEqual(mockMessages);
            expect(messagesContainer.children.length).toBe(2);
            expect(messagesContainer.children[0].className).toBe('message user');
            expect(messagesContainer.children[0].textContent).toBe('Hello');
            expect(messagesContainer.children[1].className).toBe('message assistant');
            expect(messagesContainer.children[1].textContent).toBe('Hi there');
            
            // Verify status updated
            expect(status.textContent).toBe('Ready to chat with Помощник');
        });

        test('selectAgent shows error when fetch fails', async () => {
            const mockAgent = { Name: 'Test', Description: 'Test' };
            const agentItem = document.createElement('li');
            agentItem.className = 'agent-item';
            agentsList.appendChild(agentItem);
            
            fetch.mockRejectedValueOnce(new Error('Network error'));
            
            const mockEvent = { target: agentItem };
            
            await chatApp.selectAgent(mockAgent, mockEvent);
            
            // Verify error is shown
            expect(errorBox.textContent).toBe('Failed to load messages: Network error');
            expect(errorBox.classList.contains('hidden')).toBe(false);
            
            // Verify placeholder message is shown
            expect(messagesContainer.innerHTML).toContain('No messages yet');
        });
    });

    describe('Message rendering', () => {
        test('renderMessages filters system messages', () => {
            const messages = [
                { Role: 'system', Content: 'You are a helpful assistant' },
                { Role: 'user', Content: 'Hello' },
                { Role: 'assistant', Content: 'Hi' },
                { Role: 'system', Content: 'Ignore this' },
            ];
            
            chatApp.renderMessages(messages);
            
            // Only user and assistant messages should be kept
            expect(chatApp.getCurrentMessages()).toEqual([
                { Role: 'user', Content: 'Hello' },
                { Role: 'assistant', Content: 'Hi' },
            ]);
            
            // Verify DOM contains only user and assistant messages
            expect(messagesContainer.children.length).toBe(2);
            expect(messagesContainer.children[0].className).toBe('message user');
            expect(messagesContainer.children[1].className).toBe('message assistant');
        });

        test('renderMessages shows placeholder when no messages', () => {
            chatApp.renderMessages([]);
            
            expect(messagesContainer.innerHTML).toBe(
                '<div class="message assistant">No messages yet. Start the conversation!</div>'
            );
            expect(chatApp.getCurrentMessages()).toEqual([]);
        });
    });

    describe('Sending messages', () => {
        beforeEach(() => {
            // Set up a current agent by calling selectAgent
            const mockAgent = { Name: 'Помощник', Description: 'Test' };
            const agentItem = document.createElement('li');
            agentItem.className = 'agent-item';
            agentsList.appendChild(agentItem);
            
            // Mock initial messages load
            fetch.mockResolvedValueOnce({
                ok: true,
                json: async () => [],
            });
            
            return chatApp.selectAgent(mockAgent, { target: agentItem });
        });

        test('sendMessage adds user message immediately and sends to API', async () => {
            // Set input text
            messageInput.value = 'How are you?';
            
            // Mock successful API response
            fetch.mockResolvedValueOnce({
                ok: true,
                json: async () => ({ response: 'I am fine, thank you.' }),
            });
            
            await chatApp.sendMessage();
            
            // Verify user message was added to DOM immediately
            // Find user message in container
            const userMessages = Array.from(messagesContainer.children).filter(
                child => child.className === 'message user'
            );
            expect(userMessages).toHaveLength(1);
            expect(userMessages[0].textContent).toBe('How are you?');
            // Ensure placeholder is not present
            expect(messagesContainer.innerHTML).not.toContain('No messages yet');
            
            // Verify input was cleared and disabled during request
            expect(messageInput.value).toBe('');
            
            // Verify fetch was called with correct parameters
            expect(fetch).toHaveBeenCalledWith(
                'http://localhost:8080/api/agents/%D0%9F%D0%BE%D0%BC%D0%BE%D1%89%D0%BD%D0%B8%D0%BA/messages',
                {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ message: 'How are you?' }),
                }
            );
            
            // Verify assistant response was added
            // Note: The mock fetch resolves after the test, but we can check the second call
            // Actually, we need to wait for the async operation to complete
            // Since we await sendMessage, the fetch mock should have resolved
            // The renderMessages for assistant response should have been called
            // Let's check the final state
            expect(chatApp.getCurrentMessages()).toContainEqual(
                { Role: 'assistant', Content: 'I am fine, thank you.' }
            );
            
            // Verify status updated
            expect(status.textContent).toBe('Ready to chat with Помощник');
        });

        test('sendMessage handles API error', async () => {
            messageInput.value = 'Test message';
            
            // Mock failed API call
            fetch.mockRejectedValueOnce(new Error('API error'));
            
            await chatApp.sendMessage();
            
            // Verify error is shown
            expect(errorBox.textContent).toBe('Failed to send message: API error');
            expect(errorBox.classList.contains('hidden')).toBe(false);
            
            // Verify status shows error
            expect(status.textContent).toBe('Error - try again');
            
            // Verify input was re-enabled (check that it's not disabled)
            expect(messageInput.disabled).toBe(false);
        });

        test('sendMessage does nothing when no agent selected', async () => {
            // Clear current agent by creating a new chat app without selecting agent
            const newChatApp = createChatApp({
                agentsList,
                messagesContainer,
                messageInput,
                sendButton,
                status,
                errorBox,
                API_BASE: 'http://localhost:8080/api'
            });
            
            // Verify no agent is selected
            expect(newChatApp.getCurrentAgent()).toBeNull();
            
            messageInput.value = 'Test';
            
            await newChatApp.sendMessage();
            
            expect(fetch).not.toHaveBeenCalled();
        });

        test('sendMessage does nothing with empty input', async () => {
            messageInput.value = '   ';
            expect(messageInput.value).toBe('   ');
            
            await chatApp.sendMessage();
            
            expect(fetch).not.toHaveBeenCalled();
        });
    });

    describe('Helper functions', () => {
        test('escapeHtml escapes special characters', () => {
            const result = chatApp.escapeHtml('<script>alert("xss")</script>');
            // jsdom may escape quotes differently
            expect(result).toMatch(/&lt;script&gt;alert\(.xss.\)&lt;\/script&gt;/);
        });

        test('showError displays and hides error message', () => {
            jest.useFakeTimers();
            
            chatApp.showError('Test error');
            
            expect(errorBox.textContent).toBe('Test error');
            expect(errorBox.classList.contains('hidden')).toBe(false);
            
            // Fast-forward timers
            jest.advanceTimersByTime(5000);
            expect(errorBox.classList.contains('hidden')).toBe(true);
            
            jest.useRealTimers();
        });
    });
});