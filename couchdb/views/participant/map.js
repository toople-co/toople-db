function(doc) {
	if (doc.type == 'participant') {
		emit([doc.user, doc.event], null);
	}
}
