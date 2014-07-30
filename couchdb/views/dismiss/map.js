function(doc) {
	if (doc.type == 'dismiss') {
		emit(doc.user, doc.what);
	}
}
