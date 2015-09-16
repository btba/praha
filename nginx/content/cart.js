function updateItem(itemID) {
    var quantity = parseInt(document.getElementById('items.' + itemID + '.quantity').value)
    if (isNaN(quantity)) {
	setAlert('Please enter a valid quantity');
	return;
    }
    if (quantity < 1 || quantity > 99) {
	setAlert('Please enter a quantity between 1 and 99');
	return;
    }

    var xhr = new XMLHttpRequest();
    xhr.onreadystatechange = function() {
        if (xhr.readyState == 4) {
            if (xhr.status == 200) {
		clearAlert();
	    } else {
		setAlert('Error updating item: ' + xhr.responseText);
            }
        }
    }
    xhr.open('PUT', '/reservations/api/cartitems/' + itemID, true);
    xhr.setRequestHeader('Content-Type', 'application/json');
    xhr.send(JSON.stringify(quantity));
}

function removeItem(itemID) {
    var itemRow = document.getElementById('items.' + itemID);
    prevDisplay = itemRow.style.display;
    itemRow.style.display = 'none';

    var xhr = new XMLHttpRequest();
    xhr.onreadystatechange = function() {
        if (xhr.readyState == 4) {
            if (xhr.status == 200) {
		clearAlert()
	    } else {
                setAlert('Error deleting item: ' + xhr.responseText);
		itemRow.style.display = prevDisplay;
            }
        }
    }
    xhr.open('DELETE', '/reservations/api/cartitems/' + itemID, true);
    xhr.send();
}

function setAlert(s) {
    var alert = document.getElementById('alert');
    alert.innerHTML = s; 
    alert.style.display = 'block';
}

function clearAlert() {
    var alert = document.getElementById('alert');
    alert.style.display = 'none';
}
