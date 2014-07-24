function(doc) {
	if (doc.type == 'user') {
		for (i in doc.emails) {
			emit(doc.emails[i], null);
		}
	}
}
