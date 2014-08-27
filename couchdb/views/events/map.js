function(doc) {
	if (doc.type == 'invitation') {
		emit(doc.circle, {_id: doc.event});
	}
}
