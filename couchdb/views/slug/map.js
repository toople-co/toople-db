function(doc) {
	if (doc.type == 'circle') {
		emit(doc.slug, null);
	}
}
