// Camera handling functions
let stream = null;
let video = null;
let canvas = null;

function stopCamera() {
    if (stream) {
        stream.getTracks().forEach(track => track.stop());
        stream = null;
    }
    if (video) {
        video.srcObject = null;
        video = null;
    }
}

function startCamera() {
    // Clear any existing photo
    const preview = document.getElementById('camera-preview');
    const existingPhoto = document.getElementById('existing-photo');
    if (existingPhoto) {
        existingPhoto.remove();
    }
    
    // Create and setup video element if it doesn't exist
    if (!video) {
        video = document.createElement('video');
        video.setAttribute('playsinline', '');
        video.setAttribute('autoplay', '');
        video.classList.add('w-full', 'h-full', 'object-cover');
        preview.innerHTML = '';
        preview.appendChild(video);
    }

    // Show/hide appropriate buttons
    document.getElementById('start-camera').classList.add('hidden');
    document.getElementById('capture-photo').classList.remove('hidden');
    document.getElementById('retake-photo').classList.add('hidden');

    // Start video stream
    navigator.mediaDevices.getUserMedia({ video: true, audio: false })
        .then(function(mediaStream) {
            stream = mediaStream;
            video.srcObject = stream;
            video.play();
        })
        .catch(function(err) {
            console.log("An error occurred: " + err);
        });
}

function capturePhoto() {
    if (!canvas) {
        canvas = document.createElement('canvas');
    }
    
    const preview = document.getElementById('camera-preview');
    canvas.width = video.videoWidth;
    canvas.height = video.videoHeight;
    
    // Draw the video frame to the canvas
    canvas.getContext('2d').drawImage(video, 0, 0);
    
    // Convert to data URL and set as preview
    const dataUrl = canvas.toDataURL('image/jpeg');
    document.getElementById('photo-data').value = dataUrl;
    preview.innerHTML = `<img src="${dataUrl}" class="w-full h-full object-cover">`;
    
    // Stop the video stream
    stopCamera();
    
    // Show/hide appropriate buttons
    document.getElementById('start-camera').classList.remove('hidden');
    document.getElementById('capture-photo').classList.add('hidden');
    document.getElementById('retake-photo').classList.remove('hidden');
}

function clearForm() {
    stopCamera();
    const preview = document.getElementById('camera-preview');
    preview.innerHTML = '<span class="text-gray-400">No photo</span>';
    document.getElementById('photo-data').value = '';
    document.getElementById('start-camera').classList.remove('hidden');
    document.getElementById('capture-photo').classList.add('hidden');
    document.getElementById('retake-photo').classList.add('hidden');
}

// Clean up when navigating away
document.addEventListener('htmx:beforeCleanupElement', function() {
    stopCamera();
});