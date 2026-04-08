// Test setup for Jest with jsdom

// Mock global fetch if not already mocked
if (!global.fetch) {
    global.fetch = jest.fn();
}